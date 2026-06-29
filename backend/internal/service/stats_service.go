package service

import (
	"context"
	"fmt"

	"github.com/amll-dev/amll-hub/backend/internal/repository"
)

// StatsService 词库统计服务
type StatsService struct {
	songRepo   *repository.SongRepo
	artistRepo *repository.ArtistRepo
	syncRepo   *repository.SyncRepo
}

func NewStatsService(
	songRepo *repository.SongRepo,
	artistRepo *repository.ArtistRepo,
	syncRepo *repository.SyncRepo,
) *StatsService {
	return &StatsService{
		songRepo:   songRepo,
		artistRepo: artistRepo,
		syncRepo:   syncRepo,
	}
}

// StatsResponse 词库统计响应
type StatsResponse struct {
	TotalSongs           int64            `json:"totalSongs"`
	TotalArtists         int64            `json:"totalArtists"`
	TotalAlbums          int64            `json:"totalAlbums"`
	TotalWords           int64            `json:"totalWords"`
	TotalLines           int64            `json:"totalLines"`
	PlatformDistribution map[string]int64 `json:"platformDistribution"`
	LastSyncAt           string           `json:"lastSyncAt"`
}

// GetStats 返回词库统计
func (s *StatsService) GetStats(ctx context.Context) (*StatsResponse, error) {
	totalSongs, err := s.songRepo.CountSongs(ctx)
	if err != nil {
		return nil, fmt.Errorf("count songs: %w", err)
	}
	totalArtists, err := s.artistRepo.CountArtists(ctx)
	if err != nil {
		return nil, fmt.Errorf("count artists: %w", err)
	}
	totalAlbums, err := s.songRepo.CountAlbums(ctx)
	if err != nil {
		return nil, fmt.Errorf("count albums: %w", err)
	}
	totalWords, err := s.songRepo.SumWords(ctx)
	if err != nil {
		return nil, fmt.Errorf("sum words: %w", err)
	}
	totalLines, err := s.songRepo.SumLines(ctx)
	if err != nil {
		return nil, fmt.Errorf("sum lines: %w", err)
	}
	platformDist, err := s.songRepo.PlatformDistribution(ctx)
	if err != nil {
		return nil, fmt.Errorf("platform distribution: %w", err)
	}
	lastSyncAt, _ := s.syncRepo.GetLastSyncedAt(ctx)

	return &StatsResponse{
		TotalSongs:           totalSongs,
		TotalArtists:         totalArtists,
		TotalAlbums:          totalAlbums,
		TotalWords:           totalWords,
		TotalLines:           totalLines,
		PlatformDistribution: platformDist,
		LastSyncAt:           lastSyncAt,
	}, nil
}
