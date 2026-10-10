package controllers

import (
	"autosleep/models"
	"autosleep/sleeper"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// BaseURL is the public URL of this AutoSleep deployment. SELF_HOST wins
// (with or without a scheme); otherwise it is derived from the request, so
// a fresh Heroku deploy works without any configuration.
func BaseURL(c *gin.Context) string {
	if h := sleeper.SelfHostURL(); h != "" {
		return h
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	base := scheme + "://" + c.Request.Host
	sleeper.SetPublicURL(base) // lets background jobs build drain URLs
	return base
}

func jsonError(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// AppView is an application plus the derived fields the API and dashboard
// show. Secrets are never serialised (see json:"-" tags on the model).
type AppView struct {
	models.Application
	Status      string     `json:"status"` // awake, asleep or paused
	NextSleepAt *time.Time `json:"next_sleep_at,omitempty"`
	// AwakeUntil is set while office hours keep the app awake.
	AwakeUntil   *time.Time `json:"awake_until,omitempty"`
	AccountEmail string     `json:"account_email,omitempty"`
}

func viewOf(app *models.Application) AppView {
	v := AppView{Application: *app, Status: "awake", AccountEmail: app.Account.Email}
	switch {
	case app.Paused:
		v.Status = "paused"
	case !app.CurrentStatus:
		v.Status = "asleep"
	}
	now := sleeper.Now()
	if at := sleeper.NextSleepAt(app, now); !at.IsZero() {
		v.NextSleepAt = &at
	}
	if end, ok := sleeper.KeptAwake(app, now); ok {
		v.AwakeUntil = &end
	}
	return v
}

// respondApp re-reads an app after a change and writes it as JSON, or a 404
// if it was removed meanwhile.
func respondApp(c *gin.Context, name string) {
	app, err := sleeper.Load(models.DB, name)
	if err != nil {
		jsonError(c, http.StatusNotFound, "App "+name+" is not managed by AutoSleep")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": viewOf(app)})
}

// loadApp loads :app_id or writes a 404.
func loadApp(c *gin.Context) (*models.Application, bool) {
	app, err := sleeper.Load(models.DB, c.Param("app_id"))
	if err != nil {
		jsonError(c, http.StatusNotFound, "App "+c.Param("app_id")+" is not managed by AutoSleep")
		return nil, false
	}
	return app, true
}
