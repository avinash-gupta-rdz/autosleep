package models

import (
	"time"

	"gorm.io/gorm"
)

// History actions. Action is stored as a bool for compatibility with
// existing rows: true = woke up, false = went to sleep.
const (
	ActionSleep = false
	ActionWake  = true
)

// AppHistory records one process type being put to sleep or woken up.
// Pairs of sleep/wake rows are what the savings report is computed from.
type AppHistory struct {
	gorm.Model
	ApplicationId uint      `gorm:"index:idx_history_app_time,priority:1" json:"application_id"`
	Action        bool      `json:"action"`
	DynoName      string    `json:"dyno_name"`
	DynoSize      string    `json:"dyno_size"`
	Quantity      int       `gorm:"not null;default:0" json:"quantity"`
	Reason        string    `gorm:"size:64;not null;default:''" json:"reason"`
	PerformedAt   time.Time `gorm:"index:idx_history_app_time,priority:2" json:"performed_at"`
}

// EnrollOptOut lists apps the user removed from AutoSleep, so AUTO_ENROLL
// doesn't add them back. Adding the app manually clears the entry.
type EnrollOptOut struct {
	ID            uint   `gorm:"primaryKey"`
	HerokuAppName string `gorm:"size:255;uniqueIndex"`
	CreatedAt     time.Time
}
