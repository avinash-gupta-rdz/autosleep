package models

import (
  "github.com/jinzhu/gorm"
  "time"
  "encoding/json"
  "database/sql/driver"
)
type JSONB  map[string]map[string]string// map[string]interface{}

type Application struct {
  gorm.Model
  // ID     			uint   `json:"id" gorm:"primary_key"`
  HerokuAppName  	string `json:"heroku_app_name",sql:"unique_index"`
  CurrentStatus 	bool `json:"current_status"`
  RecentActivityAt  time.Time `json:"recent_activity_at"`
  HerokuApiKey 		string `json:"heroku_api_key"`
  DrainId 		string `json:"drain_id"`
  CurrentConfig    JSONB   `sql:"type:jsonb",json:"current_config"`
  IdealTime float64   `json:"ideal_time"`
  CheckInterval int64   `json:"check_interval"`

}

 func (j JSONB) Value() (driver.Value, error) {
  valueString, err := json.Marshal(j)
  return string(valueString), err
 }
 
 func (j *JSONB) Scan(value interface{}) error {
  if err := json.Unmarshal(value.([]byte), &j); err != nil {
    return err
  }
  return nil
 }