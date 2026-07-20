package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/amll-dev/amll-hub/backend/internal/config"
	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/amll-dev/amll-hub/backend/internal/repository"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	logrus "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// LyricsService 歌词获取服务
type LyricsService struct {
	cfg      *config.Config
	songRepo *repository.SongRepo
	minio    *minio.Client
	redis    *redis.Client
}

func NewLyricsService(
	cfg *config.Config,
	songRepo *repository.SongRepo,
	minioClient *minio.Client,
	redisClient *redis.Client,
) *LyricsService {
	return &LyricsService{
		cfg:      cfg,
		songRepo: songRepo,
		minio:    minioClient,
		redis:    redisClient,
	}
}

// LyricResult 歌词获取结果
type LyricResult struct {
	// MinioPath 用于从 MinIO 取对象
	MinioPath string
	// Size 文件总大小（字节）
	Size int64
	// ETag 对象 ETag
	ETag string
}

// ResolveLyric 解析 folder+filename -> MinioPath
// 优先从 Redis 缓存读取，未命中则查 PG
func (s *LyricsService) ResolveLyric(ctx context.Context, folder, filename string) (*LyricResult, error) {
	// 1. Redis 缓存
	cacheKey := lyricCacheKey(folder, filename)
	if s.redis != nil {
		if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			// 同时获取对象 stat 信息
			objInfo, err := s.minio.StatObject(ctx, s.cfg.MinIO.Bucket, cached, minio.StatObjectOptions{})
			if err == nil {
				return &LyricResult{
					MinioPath: cached,
					Size:      objInfo.Size,
					ETag:      objInfo.ETag,
				}, nil
			}
			// 对象可能已被删除，清缓存继续查 PG
			_ = s.redis.Del(ctx, cacheKey).Err()
		}
	}

	// 2. 查 PG
	song, err := s.songRepo.GetByPlatform(ctx, folder, filename)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLyricNotFound
		}
		return nil, fmt.Errorf("query song: %w", err)
	}

	// 3. Stat 对象
	objInfo, err := s.minio.StatObject(ctx, s.cfg.MinIO.Bucket, song.MinioPath, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("stat minio object: %w", err)
	}

	// 4. 回写缓存（TTL 1 小时）
	if s.redis != nil {
		_ = s.redis.Set(ctx, cacheKey, song.MinioPath, time.Hour).Err()
	}

	return &LyricResult{
		MinioPath: song.MinioPath,
		Size:      objInfo.Size,
		ETag:      objInfo.ETag,
	}, nil
}

// StreamLyric 流式返回 TTML 内容
// rangeHeader 为 HTTP Range 头（可空）
// onWrite 在每次写入响应体时调用（用于直接 io.Copy 到 ResponseWriter）
func (s *LyricsService) StreamLyric(
	ctx context.Context,
	minioPath string,
	rangeHeader string,
	onWrite func(contentLength int64, reader io.Reader) error,
) (status int, contentRange string, contentLength int64, err error) {
	opt := minio.GetObjectOptions{}
	totalSize := int64(0)

	// 先 stat 拿到 total size
	statInfo, statErr := s.minio.StatObject(ctx, s.cfg.MinIO.Bucket, minioPath, minio.StatObjectOptions{})
	if statErr == nil {
		totalSize = statInfo.Size
	}

	if rangeHeader != "" && totalSize > 0 {
		rng := pkg.ParseRange(rangeHeader, totalSize)
		if !rng.Valid {
			return 416, "", 0, ErrInvalidRange
		}
		if err := opt.SetRange(rng.Start, rng.End); err != nil {
			return 500, "", 0, fmt.Errorf("set range: %w", err)
		}
	}

	obj, err := s.minio.GetObject(ctx, s.cfg.MinIO.Bucket, minioPath, opt)
	if err != nil {
		return 500, "", 0, fmt.Errorf("get object: %w", err)
	}
	defer func() {
		if err := obj.Close(); err != nil {
			logrus.Errorf("close minio object: %v", err)
		}
	}()

	// 获取对象实际信息（带 Range 时 Size 为分片大小）
	objInfo, err := obj.Stat()
	if err != nil {
		// 如果是 416，minio 会返回 InvalidRange
		return 416, "", 0, ErrInvalidRange
	}

	contentLength = objInfo.Size
	if rangeHeader != "" && totalSize > 0 {
		rng := pkg.ParseRange(rangeHeader, totalSize)
		if rng.Valid {
			contentRange = rng.ContentRangeHeader()
			status = 206
		} else {
			status = 200
		}
	} else {
		status = 200
	}

	if err := onWrite(contentLength, obj); err != nil {
		return 500, "", 0, err
	}
	return status, contentRange, contentLength, nil
}

// lyricCacheKey Redis 缓存键
func lyricCacheKey(folder, filename string) string {
	return "lyric:" + folder + ":" + filename
}

// ErrLyricNotFound 歌词未找到
var ErrLyricNotFound = errors.New("lyric not found")

// ErrInvalidRange Range 非法
var ErrInvalidRange = errors.New("invalid range")

// _ 防 strings/logrus 未引用
var _ = strings.TrimSpace
var _ = logrus.Infof
var _ = pkg.HTTPRangeOutOfRange
