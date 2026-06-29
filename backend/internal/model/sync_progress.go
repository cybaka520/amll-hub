package model

import "time"

// SyncProgress 同步进度表
type SyncProgress struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	SyncHistoryID int64     `gorm:"not null;index" json:"syncHistoryId"`
	Total         int       `gorm:"not null;default:0" json:"total"`
	Downloaded    int       `gorm:"not null;default:0" json:"downloaded"`
	Failed        int       `gorm:"not null;default:0" json:"failed"`
	CurrentFile   string    `gorm:"type:varchar(255)" json:"currentFile"`
	UpdatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (SyncProgress) TableName() string { return "sync_progress" }
