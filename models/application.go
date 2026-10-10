package models

import (
	"autosleep/schedule"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// JSONB maps a process type to its formation, e.g.
// {"web": {"Quantity": "2", "Size": "standard-1X"}}. It is the dyno
// configuration AutoSleep restores when an app wakes up.
type JSONB map[string]map[string]string

type Application struct {
	gorm.Model
	HerokuAppName    string     `gorm:"index:idx_heroku_app_name,unique" json:"heroku_app_name"`
	CurrentStatus    bool       `json:"current_status"` // true = awake
	RecentActivityAt time.Time  `json:"recent_activity_at"`
	HerokuApiKey     []byte     `json:"-"`
	DrainId          string     `json:"drain_id"`
	DrainToken       string     `gorm:"size:255;not null;default:''" json:"-"`
	CurrentConfig    JSONB      `json:"current_config"`
	IdealTime        float64    `json:"ideal_time"`
	CheckInterval    int64      `json:"check_interval"`
	ManualMode       bool       `json:"manual_mode"`
	NightMode        bool       `json:"night_mode"`
	Paused           bool       `gorm:"not null;default:false" json:"paused"`
	WebOnly          bool       `gorm:"not null;default:false" json:"web_only"`
	WakePage         bool       `gorm:"not null;default:false" json:"wake_page"`
	LastError        string     `gorm:"size:1024;not null;default:''" json:"last_error"`
	LastErrorAt      *time.Time `json:"last_error_at"`
	// LastDrainAt is when Heroku last delivered any log batch for the app
	// (updated at most once a minute). Nil means the drain never worked.
	LastDrainAt *time.Time `json:"last_drain_at"`

	// Office hours: the app is kept awake (and woken at the start) during
	// this weekly window. AwakeDays is e.g. "mon-fri"; empty means none.
	AwakeDays string `gorm:"size:64;not null;default:''" json:"awake_days"`
	AwakeFrom string `gorm:"size:5;not null;default:''" json:"awake_from"`
	AwakeTo   string `gorm:"size:5;not null;default:''" json:"awake_to"`
	AwakeTZ   string `gorm:"size:64;not null;default:''" json:"awake_tz"`

	AccountId *uint   `json:"account_id"`
	Account   Account `gorm:"foreignKey:AccountId" json:"-"`
}

// Value stores JSONB as a JSON string.
func (j JSONB) Value() (driver.Value, error) {
	valueString, err := json.Marshal(j)
	return string(valueString), err
}

// Scan reads JSONB from either a []byte (MySQL) or string (Postgres/SQLite).
func (j *JSONB) Scan(value interface{}) error {
	switch v := value.(type) {
	case nil:
		*j = nil
		return nil
	case []byte:
		return json.Unmarshal(v, j)
	case string:
		return json.Unmarshal([]byte(v), j)
	default:
		return fmt.Errorf("unsupported JSONB source %T", value)
	}
}

// ErrNoCredentials means an app has neither an API key nor a linked account.
var ErrNoCredentials = errors.New("app has no Heroku API key or connected account")

// GetToken returns a usable Heroku API token for the app, refreshing the
// OAuth token of the linked account when needed. app.Account must be loaded.
func (app *Application) GetToken() (string, error) {
	if len(app.HerokuApiKey) > 0 {
		return Decrypt(app.HerokuApiKey)
	}
	if app.AccountId == nil || app.Account.ID == 0 {
		return "", ErrNoCredentials
	}
	return app.Account.CurrentAccessToken()
}

// OfficeHours returns the app's keep-awake window, if it has one.
func (app *Application) OfficeHours() (schedule.Weekly, bool) {
	if app.AwakeDays == "" {
		return schedule.Weekly{}, false
	}
	w, err := schedule.ParseWeekly(app.AwakeDays, app.AwakeFrom, app.AwakeTo, app.AwakeTZ)
	return w, err == nil
}

// IdleDuration is how long the app may go without traffic before it sleeps.
func (app *Application) IdleDuration() time.Duration {
	return time.Duration(app.IdealTime * float64(time.Second))
}

// CheckEvery is how often the sleep checker re-evaluates an awake app.
func (app *Application) CheckEvery() time.Duration {
	return time.Duration(app.CheckInterval) * time.Second
}
