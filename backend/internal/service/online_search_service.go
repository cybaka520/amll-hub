package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	musicapi "github.com/xiaowumin-mark/AMLX-MUSIC-API"

	_ "github.com/xiaowumin-mark/AMLX-MUSIC-API/kugou"
	_ "github.com/xiaowumin-mark/AMLX-MUSIC-API/netease"
	_ "github.com/xiaowumin-mark/AMLX-MUSIC-API/qqmusic"

	"github.com/amll-dev/amll-hub/backend/internal/config"
)

// OnlineSearchHit 在线搜索单条结果
type OnlineSearchHit struct {
	SongName   string   `json:"songName"`
	Artists    []string `json:"artists"`
	AlbumName  string   `json:"albumName"`
	Platform   string   `json:"platform"`    // ncm / qq / kugou
	PlatformID string   `json:"platformId"`  // 平台歌曲 ID
	Duration   int      `json:"duration"`    // 时长（秒）
	CoverURL   string   `json:"coverUrl"`    // 封面 URL
}

// OnlineSearchResult 在线搜索结果
type OnlineSearchResult struct {
	Hits  []OnlineSearchHit `json:"hits"`
	Total int               `json:"total"` // 总数，-1 表示未知
}

// OnlineSearchService 在线搜索服务
type OnlineSearchService struct {
	cfg       *config.Config
	providers map[string]musicapi.MusicProvider // key: ncm/qq/kugou
}

func NewOnlineSearchService(cfg *config.Config) *OnlineSearchService {
	httpClient := &http.Client{
		Timeout: time.Duration(cfg.OnlineSearch.TimeoutSec) * time.Second,
	}

	providers := make(map[string]musicapi.MusicProvider)

	if p, err := musicapi.Get("netease", musicapi.WithHTTPClient(httpClient)); err == nil {
		providers["ncm"] = p
	}
	if p, err := musicapi.Get("qq", musicapi.WithHTTPClient(httpClient)); err == nil {
		providers["qq"] = p
	}
	if p, err := musicapi.Get("kugou", musicapi.WithHTTPClient(httpClient)); err == nil {
		providers["kugou"] = p
	}

	return &OnlineSearchService{cfg: cfg, providers: providers}
}

// Search 执行在线搜索，platform 三选一：ncm / qq / kugou
func (s *OnlineSearchService) Search(ctx context.Context, query string, platform string, limit int) (*OnlineSearchResult, error) {
	provider, ok := s.providers[platform]
	if !ok {
		return nil, fmt.Errorf("不支持的平台: %s（可选: ncm, qq, kugou）", platform)
	}

	result, err := provider.Search(ctx, query, musicapi.SearchTypeSong, 1, limit)
	if err != nil {
		return nil, fmt.Errorf("平台搜索失败: %w", err)
	}

	hits := make([]OnlineSearchHit, 0, len(result.Songs))
	for _, song := range result.Songs {
		hit := OnlineSearchHit{
			SongName:   song.Name,
			PlatformID: song.ID,
			Platform:   platform,
			Duration:   song.Duration,
			CoverURL:   song.CoverURL,
		}

		for _, a := range song.Artists {
			hit.Artists = append(hit.Artists, a.Name)
		}

		if song.Album != nil {
			hit.AlbumName = song.Album.Name
		}

		hits = append(hits, hit)
	}

	return &OnlineSearchResult{
		Hits:  hits,
		Total: result.Total,
	}, nil
}
