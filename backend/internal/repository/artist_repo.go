package repository

import (
	"context"

	"github.com/amll-dev/amll-hub/backend/internal/model"
	"gorm.io/gorm"
)

// ArtistRepo 艺术家数据访问层
type ArtistRepo struct {
	db *gorm.DB
}

func NewArtistRepo(db *gorm.DB) *ArtistRepo {
	return &ArtistRepo{db: db}
}

// CountArtists 艺术家总数
func (r *ArtistRepo) CountArtists(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Artist{}).Count(&count).Error
	return count, err
}
