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
	name := c.Param("app_id")
	now := sleeper.Now()
	// Most batches are app logs, not traffic. Skip the database for those,
	// except for a once-a-minute "logs are arriving" heartbeat.
	heartbeat := drainSeen.due(name, now)
	if !summary.Traffic && !heartbeat {
		c.JSON(http.StatusOK, gin.H{"status": false})
		return
	}
	app, err := sleeper.Load(models.DB, name)
	if err != nil {
		drainSeen.mark(name, now) // e.g. a drain left on a removed app: don't look it up on every batch
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
	if heartbeat {
		drainSeen.mark(name, now)
		models.DB.Model(&models.Application{}).Where("id = ?", app.ID).Update("last_drain_at", now)
	}
	if !summary.Traffic {
		c.JSON(http.StatusOK, gin.H{"status": false})
		return
	}

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

// drainHeartbeat is how often last_drain_at is refreshed per app.
const drainHeartbeat = time.Minute

// drainSeen throttles last_drain_at writes. Entries for names that aren't
// apps are bounded by resetting the map when it grows too large.
var drainSeen = &throttle{every: drainHeartbeat, last: map[string]time.Time{}}

type throttle struct {
	mu    sync.Mutex
	every time.Duration
	last  map[string]time.Time
}

// due reports whether key hasn't been marked within the interval.
func (t *throttle) due(key string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.last[key]
	return !ok || now.Sub(last) >= t.every || now.Before(last)
}

// mark records key as seen. Only authenticated batches are marked, so
// forged posts can't suppress a real app's heartbeat.
func (t *throttle) mark(key string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.last) > 10000 {
		t.last = map[string]time.Time{}
	}
	t.last[key] = now
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
