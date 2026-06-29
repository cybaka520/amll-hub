package repository

import (
	"context"
	"errors"

	"github.com/amll-dev/amll-hub/backend/internal/model"
	"gorm.io/gorm"
)

// SyncRepo 同步状态/历史数据访问层
type SyncRepo struct {
	db *gorm.DB
}

func NewSyncRepo(db *gorm.DB) *SyncRepo {
	return &SyncRepo{db: db}
}

// GetStateValue 读取 sync_state 表中的某个 key
func (r *SyncRepo) GetStateValue(ctx context.Context, key string) (string, error) {
	var s model.SyncState
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return s.Value, nil
}

// SetStateValue 写入 sync_state 表中的某个 key
func (r *SyncRepo) SetStateValue(ctx context.Context, key, value string) error {
	s := model.SyncState{Key: key, Value: value}
	return r.db.WithContext(ctx).Save(&s).Error
}

// GetLastSyncedCommit 读取最近同步的 commit hash
func (r *SyncRepo) GetLastSyncedCommit(ctx context.Context) (string, error) {
	return r.GetStateValue(ctx, "last_synced_commit")
}

// GetLastSyncedAt 读取最近同步时间
func (r *SyncRepo) GetLastSyncedAt(ctx context.Context) (string, error) {
	return r.GetStateValue(ctx, "last_synced_at")
}

// GetLatestRunningHistory 获取最新的 running 历史
func (r *SyncRepo) GetLatestRunningHistory(ctx context.Context) (*model.SyncHistory, error) {
	var h model.SyncHistory
	err := r.db.WithContext(ctx).
		Where("status = ?", "running").
		Order("started_at DESC").
		First(&h).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// GetLatestHistory 获取最新一条历史（任意状态）
func (r *SyncRepo) GetLatestHistory(ctx context.Context) (*model.SyncHistory, error) {
	var h model.SyncHistory
	err := r.db.WithContext(ctx).
		Order("started_at DESC").
		First(&h).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// GetHistoryByID 根据 ID 查询历史
func (r *SyncRepo) GetHistoryByID(ctx context.Context, id int64) (*model.SyncHistory, error) {
	var h model.SyncHistory
	err := r.db.WithContext(ctx).First(&h, id).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}
