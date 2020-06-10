package models

import (
  "github.com/jinzhu/gorm"
  "time"
)

type AppHistory struct {
  gorm.Model
  ApplicationId  uint `json:"application_id"`
  Application Application `gorm:"foreignkey:ApplicationId"`
  Action bool `json:"action"`
  DynoName string `json:"dyno_name"`
  DynoSize string `json:"dyno_size"`
  PerformedAt time.Time `json:"performed_at"`
}
