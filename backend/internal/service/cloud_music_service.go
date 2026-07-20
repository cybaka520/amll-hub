package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/amll-dev/amll-hub/backend/internal/config"
	"github.com/redis/go-redis/v9"
	logrus "github.com/sirupsen/logrus"

	netease "github.com/cybaka520/AMLLHub-Music-API/pkg"
	"github.com/cybaka520/AMLLHub-Music-API/pkg/models"
	"github.com/cybaka520/AMLLHub-Music-API/pkg/utils"
)

// CloudMusicService 网易云解析服务
type CloudMusicService struct {
	cfg    *config.Config
	redis  *redis.Client
	client *netease.Client
}

// NewCloudMusicService 构造函数；若 MusicU 为空则 client=nil，调用时返回错误
func NewCloudMusicService(cfg *config.Config, redisClient *redis.Client) *CloudMusicService {
	s := &CloudMusicService{cfg: cfg, redis: redisClient}
	if cfg.NCM.MusicU != "" {
		s.client = netease.NewClient(cfg.NCM.MusicU)
	} else {
		logrus.Warn("NCM_MUSIC_U not set, cloud music parse API will be disabled")
	}
	return s
}

// Search 搜索音乐
// keywords: 搜索关键词，limit: 返回数量（1-100）
// 缓存 key: ncm:search:{keywords}:{limit}，TTL 10 分钟
// 上游返回业务错误时不写入缓存
func (s *CloudMusicService) Search(ctx context.Context, keywords string, limit int) (*models.SearchResponse, error) {
	if s.client == nil {
		return nil, fmt.Errorf("NCM_MUSIC_U 未配置")
	}
	if keywords = strings.TrimSpace(keywords); keywords == "" {
		return nil, fmt.Errorf("搜索关键词不能为空")
	}
	if limit < 1 {
		limit = 1
	} else if limit > 100 {
		limit = 100
	}

	cacheKey := fmt.Sprintf("ncm:search:%s:%d", keywords, limit)
	if s.redis != nil {
		if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var r models.SearchResponse
			if err := json.Unmarshal([]byte(cached), &r); err == nil {
				return &r, nil
			}
		}
	}

	resp, err := s.client.Search(ctx, keywords, limit)
	if err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}

	// 上游业务错误直接透传，不缓存
	if resp.Code != 200 || resp.Error != "" {
		if resp.Error != "" {
			return resp, fmt.Errorf("%s", resp.Error)
		}
		return resp, fmt.Errorf("上游返回错误码: %d", resp.Code)
	}

	if s.redis != nil {
		if data, err := json.Marshal(resp); err == nil {
			s.redis.Set(ctx, cacheKey, data, 10*time.Minute)
		}
	}
	return resp, nil
}

// ParseMusic 通过音乐 ID 解析单曲（含音质选择）
// songID 支持纯数字、music.163.com URL、163cn.tv 短链接（短链接需调用方先重定向）
// 缓存 key: ncm:parse-music:{normalizedSongID}:{level}，TTL 20 分钟
// 上游返回业务错误（Code != 200 或 Error 非空）时不写入缓存
func (s *CloudMusicService) ParseMusic(ctx context.Context, songID, level string) (*models.MusicResponse, error) {
	if s.client == nil {
		return nil, fmt.Errorf("NCM_MUSIC_U 未配置")
	}
	if !isValidLevel(level) {
		return nil, fmt.Errorf("非法音质等级: %s", level)
	}

	// 规范化 ID：从 URL 中提取纯 ID 作为缓存键，避免同资源多 key
	normalizedID, err := utils.ExtractSongID(songID)
	if err != nil {
		return nil, fmt.Errorf("无效的 songId: %w", err)
	}

	cacheKey := fmt.Sprintf("ncm:parse-music:%s:%s", normalizedID, level)
	if s.redis != nil {
		if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var m models.MusicResponse
			if err := json.Unmarshal([]byte(cached), &m); err == nil {
				return &m, nil
			}
		}
	}

	resp, err := s.client.ParseMusic(ctx, songID, level)
	if err != nil {
		return nil, fmt.Errorf("解析单曲失败: %w", err)
	}

	// 上游业务错误直接透传，不缓存
	if resp.Code != 200 || resp.Error != "" {
		if resp.Error != "" {
			return resp, fmt.Errorf("%s", resp.Error)
		}
		return resp, fmt.Errorf("上游返回错误码: %d", resp.Code)
	}

	if s.redis != nil {
		if data, err := json.Marshal(resp); err == nil {
			s.redis.Set(ctx, cacheKey, data, 20*time.Minute)
		}
	}
	return resp, nil
}

// ParsePlaylist 解析歌单（返回歌单详情与完整歌曲列表）
// playlistID 支持纯数字、带 id= 参数的 URL、/playlist/{id} 路径的 URL
// 缓存 key: ncm:parse-playlist:{normalizedPlaylistID}，TTL 30 分钟
// 上游返回业务错误时不写入缓存
func (s *CloudMusicService) ParsePlaylist(ctx context.Context, playlistID string) (*models.PlaylistResponse, error) {
	if s.client == nil {
		return nil, fmt.Errorf("NCM_MUSIC_U 未配置")
	}

	// 规范化 ID
	normalizedID, err := utils.ExtractPlaylistID(playlistID)
	if err != nil {
		return nil, fmt.Errorf("无效的 playlistId: %w", err)
	}

	cacheKey := fmt.Sprintf("ncm:parse-playlist:%s", normalizedID)
	if s.redis != nil {
		if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var p models.PlaylistResponse
			if err := json.Unmarshal([]byte(cached), &p); err == nil {
				return &p, nil
			}
		}
	}

	resp, err := s.client.ParsePlaylist(ctx, playlistID)
	if err != nil {
		return nil, fmt.Errorf("解析歌单失败: %w", err)
	}

	// 上游业务错误直接透传，不缓存
	if resp.Code != 200 || resp.Error != "" {
		if resp.Error != "" {
			return resp, fmt.Errorf("%s", resp.Error)
		}
		return resp, fmt.Errorf("上游返回错误码: %d", resp.Code)
	}

	if s.redis != nil {
		if data, err := json.Marshal(resp); err == nil {
			s.redis.Set(ctx, cacheKey, data, 30*time.Minute)
		}
	}
	return resp, nil
}

// isValidLevel 校验音质等级
func isValidLevel(level string) bool {
	switch level {
	case "standard", "exhigh", "lossless", "hires",
		"jyeffect", "jymaster", "sky", "dolby":
		return true
	}
	return false
}
