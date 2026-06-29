package repository

import (
	"context"
	"errors"

	"github.com/amll-dev/amll-hub/backend/internal/model"
	"gorm.io/gorm"
)

// SyncProgressRepo 同步进度数据访问层
type SyncProgressRepo struct {
	db *gorm.DB
}

func NewSyncProgressRepo(db *gorm.DB) *SyncProgressRepo {
	return &SyncProgressRepo{db: db}
}

// GetLatestProgress 获取最新的进度记录（按 id 降序）
func (r *SyncProgressRepo) GetLatestProgress(ctx context.Context) (*model.SyncProgress, error) {
	var p model.SyncProgress
	err := r.db.WithContext(ctx).Order("id DESC").First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProgressByHistoryID 根据 sync_history_id 查询进度
func (r *SyncProgressRepo) GetProgressByHistoryID(ctx context.Context, historyID int64) (*model.SyncProgress, error) {
	var p model.SyncProgress
	err := r.db.WithContext(ctx).
		Where("sync_history_id = ?", historyID).
		Order("id DESC").
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}
