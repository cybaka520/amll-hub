package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/amll-dev/amll-hub/backend/internal/model"
	"gorm.io/gorm"
)

// SongRepo 歌曲数据访问层
type SongRepo struct {
	db *gorm.DB
}

func NewSongRepo(db *gorm.DB) *SongRepo {
	return &SongRepo{db: db}
}

// GetByPlatform 通过 folder+filename 查找歌曲
// folder 可能是 raw-lyrics / ncm-lyrics / qq-lyrics / spotify-lyrics / am-lyrics
// - raw-lyrics：直接以 raw_lyric_file 匹配 filename
// - 平台：通过 platform_mappings 查询
func (r *SongRepo) GetByPlatform(ctx context.Context, folder, filename string) (*model.Song, error) {
	switch folder {
	case "raw-lyrics":
		var song model.Song
		if err := r.db.WithContext(ctx).
			Where("raw_lyric_file = ?", filename).
			First(&song).Error; err != nil {
			return nil, err
		}
		return &song, nil
	case "ncm-lyrics", "qq-lyrics", "spotify-lyrics", "am-lyrics":
		platform := platformFromFolder(folder)
		var song model.Song
		err := r.db.WithContext(ctx).
			Joins("JOIN platform_mappings ON platform_mappings.song_id = songs.id").
			Where("platform_mappings.platform = ? AND platform_mappings.platform_id = ?", platform, filename).
			// 取最新版本：按 commit_timestamp 降序（从 raw_lyric_file 文件名解析的提交时间戳）
			// NULLS LAST 兼容历史数据，无时间戳时退化为按 id 降序
			Order("songs.commit_timestamp DESC NULLS LAST, songs.id DESC").
			First(&song).Error
		if err != nil {
			return nil, err
		}
		return &song, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// GetByRawLyricFile 通过 raw_lyric_file 查找歌曲
func (r *SongRepo) GetByRawLyricFile(ctx context.Context, rawFile string) (*model.Song, error) {
	var song model.Song
	if err := r.db.WithContext(ctx).Where("raw_lyric_file = ?", rawFile).First(&song).Error; err != nil {
		return nil, err
	}
	return &song, nil
}

// BatchGetByPlatform 批量查询：返回 platform_id -> Song
func (r *SongRepo) BatchGetByPlatform(ctx context.Context, platform string, ids []string) (map[string]*model.Song, error) {
	if len(ids) == 0 {
		return map[string]*model.Song{}, nil
	}
	type row struct {
		PlatformID string
		model.Song
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("songs").
		Select("platform_mappings.platform_id AS platform_id, songs.*").
		Joins("JOIN platform_mappings ON platform_mappings.song_id = songs.id").
		Where("platform_mappings.platform = ? AND platform_mappings.platform_id IN ?", platform, ids).
		// 按 commit_timestamp 降序（NULLS LAST 兼容历史数据），id DESC 作为兜底
		Order("songs.commit_timestamp DESC NULLS LAST, songs.id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// 每个 platform_id 仅保留最新一条（已按时间倒序排列，第一条即为最新版本）
	result := make(map[string]*model.Song, len(rows))
	for i := range rows {
		if _, exists := result[rows[i].PlatformID]; !exists {
			s := rows[i].Song
			result[rows[i].PlatformID] = &s
		}
	}
	return result, nil
}

// GetArtistsBySongID 查询歌曲的艺术家列表
func (r *SongRepo) GetArtistsBySongID(ctx context.Context, songID int64) ([]model.Artist, error) {
	var artists []model.Artist
	err := r.db.WithContext(ctx).
		Joins("JOIN song_artists ON song_artists.artist_id = artists.id").
		Where("song_artists.song_id = ?", songID).
		Find(&artists).Error
	return artists, err
}

// GetPlatformMappingsBySongID 查询歌曲的平台映射
func (r *SongRepo) GetPlatformMappingsBySongID(ctx context.Context, songID int64) ([]model.PlatformMapping, error) {
	var pms []model.PlatformMapping
	err := r.db.WithContext(ctx).Where("song_id = ?", songID).Find(&pms).Error
	return pms, err
}

// CountSongs 歌曲总数
func (r *SongRepo) CountSongs(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Song{}).Count(&count).Error
	return count, err
}

// SumWords 总字数
func (r *SongRepo) SumWords(ctx context.Context) (int64, error) {
	var sum int64
	err := r.db.WithContext(ctx).
		Model(&model.Song{}).
		Select("COALESCE(SUM(word_count), 0)").
		Scan(&sum).Error
	return sum, err
}

// SumLines 总行数
func (r *SongRepo) SumLines(ctx context.Context) (int64, error) {
	var sum int64
	err := r.db.WithContext(ctx).
		Model(&model.Song{}).
		Select("COALESCE(SUM(line_count), 0)").
		Scan(&sum).Error
	return sum, err
}

// CountAlbums 相异专辑数（基于 JSONB 数组展开）
func (r *SongRepo) CountAlbums(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Raw(`SELECT COUNT(DISTINCT elem) FROM songs, jsonb_array_elements_text(album) AS elem WHERE elem <> ''`).
		Scan(&count).Error
	return count, err
}

// PlatformDistribution 各平台歌曲数
func (r *SongRepo) PlatformDistribution(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Platform string
		Count    int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("platform_mappings").
		Select("platform, COUNT(*) AS count").
		Group("platform").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(rows))
	for i := range rows {
		m[rows[i].Platform] = rows[i].Count
	}
	return m, nil
}

func platformFromFolder(folder string) string {
	switch folder {
	case "ncm-lyrics":
		return "ncm"
	case "qq-lyrics":
		return "qq"
	case "spotify-lyrics":
		return "spotify"
	case "am-lyrics":
		return "apple"
	}
	return ""
}

// ErrSongNotFound 歌曲未找到
var ErrSongNotFound = errors.New("song not found")

// _ 防止 fmt 在某些编译配置下报 unused
var _ = fmt.Sprintf
