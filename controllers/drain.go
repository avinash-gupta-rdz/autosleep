package controllers

import (
	"autosleep/activity"
	"autosleep/metrics"
	"autosleep/models"
	"autosleep/sleeper"
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxDrainBody = 1 << 20 // Logplex batches are far smaller
	// activityResolution limits recent_activity_at writes for busy apps.
	activityResolution = 15 * time.Second
)

// WakeInBackground lets the drain answer Logplex immediately while the app
// scales up. Tests turn it off to observe the wake synchronously.
var WakeInBackground = true

// ProcessDrain receives Heroku log drain batches. Any router log line means
// the app received an HTTP request: refresh its activity time, and wake it
// if it is asleep.
func ProcessDrain(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxDrainBody))
	if err != nil {
		jsonError(c, http.StatusBadRequest, "could not read body")
		return
	}
	summary := activity.Analyze(body)
	if !summary.Traffic {
		c.JSON(http.StatusOK, gin.H{"status": false})
		return
	}
	app, err := sleeper.Load(models.DB, c.Param("app_id"))
	if err != nil {
		metrics.DrainRejected.WithLabelValues("unknown_app").Inc()
		jsonError(c, http.StatusNotFound, "unknown app")
		return
	}
	// Heroku sends the drain's token with every batch. Apps added before
	// tokens were stored have none until the reconciler backfills it.
	if app.DrainToken != "" {
		got := c.GetHeader("Logplex-Drain-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(app.DrainToken)) != 1 {
			metrics.DrainRejected.WithLabelValues("bad_token").Inc()
			jsonError(c, http.StatusForbidden, "invalid drain token")
			return
		}
	}

	now := sleeper.Now()
	models.DB.Model(&models.Application{}).
		Where("id = ? AND recent_activity_at < ?", app.ID, now.Add(-activityResolution)).
		Update("recent_activity_at", now)

	switch {
	case !app.CurrentStatus && (!app.ManualMode || app.Paused):
		// Paused apps must run, so they're woken even in manual mode.
		background(func(ctx context.Context) { sleeper.Wake(ctx, models.DB, app, sleeper.ReasonTraffic) })
	case app.CurrentStatus && summary.NoDynos && repairDue(app.ID, now):
		// Marked awake but the router found no web dynos: heal it.
		background(func(ctx context.Context) { sleeper.Repair(ctx, models.DB, app) })
	}
	c.JSON(http.StatusOK, gin.H{"status": true})
}

// Pending tracks background wake-ups so shutdown can wait for them.
var Pending sync.WaitGroup

func background(f func(ctx context.Context)) {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*herokuTimeout)
		defer cancel()
		f(ctx)
	}
	if !WakeInBackground {
		run()
		return
	}
	Pending.Add(1)
	go func() {
		defer Pending.Done()
		run()
	}()
}

// repairInterval limits repair attempts, since every request to a broken
// app produces an H14 line.
const repairInterval = time.Minute

var (
	repairMu   sync.Mutex
	lastRepair = map[uint]time.Time{}
)

func repairDue(appID uint, now time.Time) bool {
	repairMu.Lock()
	defer repairMu.Unlock()
	if last, ok := lastRepair[appID]; ok && now.Sub(last) < repairInterval {
		return false
	}
	lastRepair[appID] = now
	return true
}
