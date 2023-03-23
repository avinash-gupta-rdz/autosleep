package models

import (
	"database/sql/driver"
	"encoding/json"
	"github.com/jinzhu/gorm"
	"time"
)

type JSONB map[string]map[string]string // map[string]interface{}

type Application struct {
	gorm.Model
	// ID     			uint   `json:"id" gorm:"primary_key"`
	HerokuAppName    string    `gorm:"index:idx_heroku_app_name,unique",json:"heroku_app_name"`
	CurrentStatus    bool      `json:"current_status"`
	RecentActivityAt time.Time `json:"recent_activity_at"`
	HerokuApiKey     []byte    `json:"heroku_api_key"`
	DrainId          string    `json:"drain_id"`
	CurrentConfig    JSONB     `json:"current_config",sql:"type:jsonb"`
	IdealTime        float64   `json:"ideal_time"`
	CheckInterval    int64     `json:"check_interval"`
	ManualMode       bool      `json:"manual_mode"`
	NightMode        bool      `json:"night_mode"`

	AccountId uint    `json:"account_id"`
	Account   Account `gorm:"foreignkey:AccountId;references:ID""`
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

func (app *Application) GetToken() string {
	if app.HerokuApiKey != nil {
		return Decrypt(app.HerokuApiKey)
	} else {
		return app.Account.CurrentAccessToken()
	}
}
