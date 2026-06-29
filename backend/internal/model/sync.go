package model

import "time"

// SyncHistory 同步历史表
type SyncHistory struct {
	ID             int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	StartedAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"startedAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	PreviousCommit string     `gorm:"column:previous_commit;type:varchar(40)" json:"previousCommit"`
	TargetCommit   string     `gorm:"column:target_commit;type:varchar(40);not null" json:"targetCommit"`
	Status         string     `gorm:"type:varchar(20);not null" json:"status"` // running, success, failed
	AddedCount     int        `gorm:"not null;default:0" json:"addedCount"`
	UpdatedCount   int        `gorm:"not null;default:0" json:"updatedCount"`
	DeletedCount   int        `gorm:"not null;default:0" json:"deletedCount"`
	ErrorMessage   string     `gorm:"type:text" json:"errorMessage"`
	TriggeredBy    string     `gorm:"type:varchar(20);not null" json:"triggeredBy"` // api, cron, github_action
	CreatedAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`

	Progress *SyncProgress `gorm:"foreignKey:SyncHistoryID" json:"progress,omitempty"`
}

func (SyncHistory) TableName() string { return "sync_history" }
