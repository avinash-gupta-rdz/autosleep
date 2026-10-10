package controllers

import (
	"autosleep/alerts"
	"autosleep/constants"
	"autosleep/models"
	"autosleep/sleeper"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SetupStatus drives the dashboard's first-run checklist: each field is one
// step a new user has (or hasn't) completed.
type SetupStatus struct {
	OAuthConfigured bool     `json:"oauth_configured"`
	Accounts        int64    `json:"accounts"`
	Apps            int64    `json:"apps"`
	LogsReceived    bool     `json:"logs_received"` // any app's drain delivered logs
	SleptOnce       bool     `json:"slept_once"`    // any app was put to sleep
	PublicURL       string   `json:"public_url"`
	SelfHostSet     bool     `json:"self_host_set"`
	AlertsEnabled   bool     `json:"alerts_enabled"`
	AutoEnroll      []string `json:"auto_enroll,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

// Setup reports first-run progress and configuration problems.
func Setup(c *gin.Context) {
	base := BaseURL(c)
	s := SetupStatus{
		OAuthConfigured: constants.OauthConfig.ClientID != "" && constants.OauthConfig.ClientSecret != "",
		PublicURL:       base,
		SelfHostSet:     sleeper.SelfHostURL() != "",
		AlertsEnabled:   alerts.Enabled(),
		AutoEnroll:      sleeper.AutoEnrollPatterns(),
	}
	models.DB.Model(&models.Account{}).Count(&s.Accounts)
	models.DB.Model(&models.Application{}).Count(&s.Apps)
	var n int64
	models.DB.Model(&models.Application{}).Where("last_drain_at IS NOT NULL").Count(&n)
	s.LogsReceived = n > 0
	models.DB.Model(&models.AppHistory{}).Where("action = ?", models.ActionSleep).Limit(1).Count(&n)
	s.SleptOnce = n > 0

	if len(s.AutoEnroll) > 0 && s.Accounts == 0 {
		s.Warnings = append(s.Warnings, "AUTO_ENROLL is set but no Heroku account is connected, so nothing can be enrolled.")
	}
	if len(s.AutoEnroll) > 0 && !s.SelfHostSet {
		s.Warnings = append(s.Warnings, "Set SELF_HOST so review apps are enrolled right after a restart, even before anyone opens the dashboard.")
	}
	if strings.HasPrefix(base, "http://") && !isLocal(c.Request.Host) {
		s.Warnings = append(s.Warnings, "AutoSleep is being reached over plain HTTP. Heroku log drains need a public HTTPS URL.")
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

func isLocal(host string) bool {
	for _, p := range []string{"localhost", "127.0.0.1", "[::1]", "0.0.0.0"} {
		if strings.HasPrefix(host, p) {
			return true
		}
	}
	return false
}
