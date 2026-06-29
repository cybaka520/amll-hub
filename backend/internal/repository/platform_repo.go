package repository

import (
	"context"

	"github.com/amll-dev/amll-hub/backend/internal/model"
	"gorm.io/gorm"
)

// PlatformRepo 平台映射数据访问层
type PlatformRepo struct {
	db *gorm.DB
}

func NewPlatformRepo(db *gorm.DB) *PlatformRepo {
	return &PlatformRepo{db: db}
}

// GetByPlatformID 通过平台+ID 查询映射
func (r *PlatformRepo) GetByPlatformID(ctx context.Context, platform, platformID string) (*model.PlatformMapping, error) {
	var pm model.PlatformMapping
	err := r.db.WithContext(ctx).
		Where("platform = ? AND platform_id = ?", platform, platformID).
		First(&pm).Error
	if err != nil {
		return nil, err
	}
	return &pm, nil
}
