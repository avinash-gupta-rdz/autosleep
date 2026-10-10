// Package sleeper puts Heroku apps to sleep and wakes them up again.
//
// State changes use compare-and-set updates on applications.current_status,
// so concurrent drain requests, checker jobs and API calls can't double-scale
// an app, and an app never goes to sleep right after traffic arrives.
package sleeper

import (
	"autosleep/herokuclient"
	"autosleep/metrics"
	"autosleep/models"
	"autosleep/schedule"
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// Reasons recorded in history.
const (
	ReasonIdle    = "idle"
	ReasonTraffic = "traffic"
	ReasonManual  = "manual"
	ReasonRemoved = "removed"
	ReasonResumed = "resumed"
	ReasonRepair  = "repair"
	// ReasonSchedule is a wake at the start of the app's office hours.
	ReasonSchedule = "schedule"
)

// KeptAwake reports whether office hours keep the app awake at now, and
// until when.
func KeptAwake(app *models.Application, now time.Time) (time.Time, bool) {
	w, ok := app.OfficeHours()
	if !ok {
		return time.Time{}, false
	}
	_, end, in := w.Current(now)
	return end, in
}

// Scheduler queues the next sleep check for an app.
type Scheduler interface {
	ScheduleCheck(appName string, in time.Duration) error
}

// Checks is the scheduler used after wake-ups. main wires it to the job
// queue; it is nil-safe so the package can be used without Redis in tests.
var Checks Scheduler

// Night is the window night-mode apps are allowed to sleep in.
var Night = schedule.Default

// Now is the clock, replaceable in tests.
var Now = func() time.Time { return time.Now().UTC() }

// MinCheckDelay keeps the checker from spinning on misconfigured apps.
const MinCheckDelay = 30 * time.Second

func client(app *models.Application) (herokuclient.Client, error) {
	token, err := app.GetToken()
	if err != nil {
		return nil, err
	}
	return herokuclient.New(token), nil
}

// Load fetches an app by Heroku name with its account preloaded.
func Load(db *gorm.DB, name string) (*models.Application, error) {
	var app models.Application
	if err := db.Preload("Account").Where("heroku_app_name = ?", name).First(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func sleepTargets(app *models.Application, formations []herokuclient.Formation) []herokuclient.Formation {
	var out []herokuclient.Formation
	for _, f := range formations {
		if f.Quantity <= 0 {
			continue
		}
		if app.WebOnly && f.Type != "web" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// ConfigFrom snapshots formations into the stored JSON config.
func ConfigFrom(formations []herokuclient.Formation) models.JSONB {
	cfg := models.JSONB{}
	for _, f := range formations {
		cfg[f.Type] = map[string]string{"Quantity": strconv.Itoa(f.Quantity), "Size": f.Size}
	}
	return cfg
}

// FormationsFrom turns the stored config back into formations, skipping
// entries with nothing to restore.
func FormationsFrom(cfg models.JSONB) []herokuclient.Formation {
	var out []herokuclient.Formation
	for t, f := range cfg {
		qty, err := strconv.Atoi(f["Quantity"])
		if err != nil || qty <= 0 {
			continue
		}
		out = append(out, herokuclient.Formation{Type: t, Size: f["Size"], Quantity: qty})
	}
	return out
}

func recordError(db *gorm.DB, app *models.Application, err error) {
	now := Now()
	msg := err.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	app.LastError, app.LastErrorAt = msg, &now
	db.Model(&models.Application{}).Where("id = ?", app.ID).
		Updates(map[string]interface{}{"last_error": msg, "last_error_at": now})
	metrics.AppErrors.Inc()
	log.Printf("autosleep: %s: %v", app.HerokuAppName, err)
}

func clearError(db *gorm.DB, app *models.Application) {
	if app.LastError == "" {
		return
	}
	app.LastError, app.LastErrorAt = "", nil
	db.Model(&models.Application{}).Where("id = ?", app.ID).
		Updates(map[string]interface{}{"last_error": "", "last_error_at": nil})
}

func recordHistory(db *gorm.DB, app *models.Application, action bool, reason string, formations []herokuclient.Formation) {
	now := Now()
	for _, f := range formations {
		h := models.AppHistory{ApplicationId: app.ID, Action: action, DynoName: f.Type, DynoSize: f.Size,
			Quantity: f.Quantity, Reason: reason, PerformedAt: now}
		if err := db.Create(&h).Error; err != nil {
			log.Printf("autosleep: %s: saving history: %v", app.HerokuAppName, err)
		}
	}
}

// Sleep scales the app's dynos to zero after saving their configuration.
// Unless force is set it only proceeds if the app is still idle. It reports
// whether the app was put to sleep.
func Sleep(ctx context.Context, db *gorm.DB, app *models.Application, reason string, force bool) (bool, error) {
	hc, err := client(app)
	if err != nil {
		recordError(db, app, err)
		return false, err
	}
	formations, err := hc.ListFormations(ctx, app.HerokuAppName)
	if err != nil {
		err = fmt.Errorf("reading dyno formation: %w", err)
		recordError(db, app, err)
		return false, err
	}
	targets := sleepTargets(app, formations)

	updates := map[string]interface{}{"current_status": false}
	if len(targets) > 0 {
		// Only overwrite the saved config when something is running; if the
		// app was already scaled to zero we must keep what we know.
		updates["current_config"] = ConfigFrom(targets)
	}
	q := db.Model(&models.Application{}).Where("id = ? AND current_status = ? AND paused = ?", app.ID, true, false)
	if !force {
		q = q.Where("recent_activity_at <= ?", Now().Add(-app.IdleDuration()))
	}
	res := q.Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil // already asleep, or traffic arrived meanwhile
	}

	down := make([]herokuclient.Formation, 0, len(targets))
	for _, f := range targets {
		down = append(down, herokuclient.Formation{Type: f.Type, Quantity: 0})
	}
	if err := hc.UpdateFormations(ctx, app.HerokuAppName, down); err != nil {
		// The request may have reached Heroku even though we got an error
		// (e.g. a timeout). Only revert if the dynos are really still up.
		if live, lerr := hc.ListFormations(ctx, app.HerokuAppName); lerr == nil && len(sleepTargets(app, live)) == 0 {
			log.Printf("autosleep: %s: scale-down reported %v but dynos are stopped; treating as asleep", app.HerokuAppName, err)
		} else {
			db.Model(&models.Application{}).Where("id = ?", app.ID).Update("current_status", true)
			err = fmt.Errorf("scaling down: %w", err)
			recordError(db, app, err)
			return false, err
		}
	}
	app.CurrentStatus = false
	if len(targets) > 0 {
		app.CurrentConfig = ConfigFrom(targets)
	}
	clearError(db, app)
	recordHistory(db, app, models.ActionSleep, reason, targets)
	metrics.ScaleEvents.WithLabelValues("sleep", reason).Inc()
	log.Printf("autosleep: %s: asleep (%s), saved formation %v", app.HerokuAppName, reason, app.CurrentConfig)
	return true, nil
}

// Wake restores the saved dyno configuration. It reports whether this call
// woke the app (false if it was already awake).
func Wake(ctx context.Context, db *gorm.DB, app *models.Application, reason string) (bool, error) {
	now := Now()
	res := db.Model(&models.Application{}).Where("id = ? AND current_status = ?", app.ID, false).
		Updates(map[string]interface{}{"current_status": true, "recent_activity_at": now})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	// Re-read so we restore the config written by the matching Sleep.
	if fresh, err := Load(db, app.HerokuAppName); err == nil {
		*app = *fresh
	}
	app.CurrentStatus, app.RecentActivityAt = true, now

	revert := func(err error) (bool, error) {
		db.Model(&models.Application{}).Where("id = ?", app.ID).Update("current_status", false)
		app.CurrentStatus = false
		recordError(db, app, err)
		return false, err
	}
	hc, err := client(app)
	if err != nil {
		return revert(err)
	}
	up := FormationsFrom(app.CurrentConfig)
	if len(up) == 0 {
		log.Printf("autosleep: %s: woke up but no saved formation to restore", app.HerokuAppName)
	}
	if err := hc.UpdateFormations(ctx, app.HerokuAppName, up); err != nil {
		return revert(fmt.Errorf("scaling up: %w", err))
	}
	clearError(db, app)
	recordHistory(db, app, models.ActionWake, reason, up)
	metrics.ScaleEvents.WithLabelValues("wake", reason).Inc()
	log.Printf("autosleep: %s: awake (%s)", app.HerokuAppName, reason)
	ScheduleNextCheck(app)
	return true, nil
}

// NextCheckIn returns how long to wait before re-checking an awake app:
// exactly when it would become idle, but no later than its check interval.
func NextCheckIn(app *models.Application, now time.Time) time.Duration {
	every := app.CheckEvery()
	if every < MinCheckDelay {
		every = MinCheckDelay
	}
	if app.NightMode && !Night.Contains(now) {
		return every
	}
	remaining := app.RecentActivityAt.Add(app.IdleDuration()).Sub(now)
	if end, ok := KeptAwake(app, now); ok {
		remaining = end.Sub(now)
	}
	if remaining < MinCheckDelay {
		remaining = MinCheckDelay
	}
	if remaining < every {
		return remaining
	}
	return every
}

// NextSleepAt predicts when an awake app will go to sleep if no traffic
// arrives, for display on the dashboard. Zero means "not scheduled".
func NextSleepAt(app *models.Application, now time.Time) time.Time {
	if !app.CurrentStatus || app.Paused {
		return time.Time{}
	}
	at := app.RecentActivityAt.Add(app.IdleDuration())
	if at.Before(now) {
		at = now
	}
	if end, ok := KeptAwake(app, at); ok {
		at = end
	}
	if app.NightMode {
		// Step forward until we're inside the night window (at most a day).
		for i := 0; i < 24*60 && !Night.Contains(at); i++ {
			at = at.Add(time.Minute)
		}
	}
	return at
}

// ScheduleNextCheck queues the next sleep check for an awake app.
func ScheduleNextCheck(app *models.Application) {
	if Checks == nil || !app.CurrentStatus || app.Paused {
		return
	}
	scheduleIn(app, NextCheckIn(app, Now()))
}

func scheduleIn(app *models.Application, in time.Duration) {
	if Checks == nil {
		return
	}
	if in < MinCheckDelay {
		in = MinCheckDelay
	}
	if err := Checks.ScheduleCheck(app.HerokuAppName, in); err != nil {
		log.Printf("autosleep: %s: scheduling check: %v", app.HerokuAppName, err)
	}
}

// Check is the sleep-checker job body. It puts the app to sleep when it has
// been idle long enough, otherwise schedules the next check.
func Check(ctx context.Context, db *gorm.DB, name string) error {
	app, err := Load(db, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // app was removed
		}
		return err
	}
	if !app.CurrentStatus || app.Paused {
		return nil // a wake-up or resume schedules the next check
	}
	now := Now()
	idle := now.Sub(app.RecentActivityAt) >= app.IdleDuration()
	_, office := KeptAwake(app, now)
	if idle && !office && (!app.NightMode || Night.Contains(now)) {
		slept, err := Sleep(ctx, db, app, ReasonIdle, false)
		if slept {
			return nil
		}
		if herokuclient.IsNotFound(err) {
			// The Heroku app was deleted (e.g. a closed review app).
			log.Printf("autosleep: %s: app no longer exists on Heroku, forgetting it", app.HerokuAppName)
			return Forget(db, app)
		}
		if err != nil {
			// Don't hammer Heroku while it (or the token) is failing.
			scheduleIn(app, app.CheckEvery())
			return nil
		}
		// Traffic raced us; reload so the next check is timed right.
		if fresh, lerr := Load(db, name); lerr == nil {
			app = fresh
		}
	}
	ScheduleNextCheck(app)
	return nil
}

// WakeForOfficeHours wakes an asleep app whose office hours have started
// since it went to sleep. An app put to sleep by hand during office hours
// stays asleep. It reports whether the app was woken.
func WakeForOfficeHours(ctx context.Context, db *gorm.DB, app *models.Application, now time.Time) (bool, error) {
	if app.CurrentStatus || app.Paused {
		return false, nil
	}
	w, ok := app.OfficeHours()
	if !ok {
		return false, nil
	}
	start, _, in := w.Current(now)
	if !in {
		return false, nil
	}
	var last models.AppHistory
	err := db.Where("application_id = ? AND action = ?", app.ID, models.ActionSleep).
		Order("performed_at desc").First(&last).Error
	if err == nil && !last.PerformedAt.Before(start) {
		return false, nil
	}
	return Wake(ctx, db, app, ReasonSchedule)
}

// Forget deletes an app and its history from AutoSleep's database. It does
// not touch Heroku.
func Forget(db *gorm.DB, app *models.Application) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("application_id = ?", app.ID).Delete(&models.AppHistory{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&models.Application{}, app.ID).Error
	})
}

// Repair restores the saved formation of an app that AutoSleep believes is
// awake but whose processes are stopped (the router reported H14). This can
// happen if a scale-down and a wake-up race, or a wake-up was interrupted.
// It reports whether anything was scaled up.
func Repair(ctx context.Context, db *gorm.DB, app *models.Application) (bool, error) {
	if !app.CurrentStatus {
		return false, nil
	}
	hc, err := client(app)
	if err != nil {
		return false, err
	}
	live, err := hc.ListFormations(ctx, app.HerokuAppName)
	if err != nil {
		return false, err
	}
	running := map[string]int{}
	for _, f := range live {
		running[f.Type] = f.Quantity
	}
	var missing []herokuclient.Formation
	for _, f := range FormationsFrom(app.CurrentConfig) {
		if running[f.Type] == 0 {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}
	// Re-check: if the app went to sleep meanwhile, a normal wake handles it.
	var fresh models.Application
	if err := db.Select("current_status").First(&fresh, app.ID).Error; err != nil || !fresh.CurrentStatus {
		return false, err
	}
	if err := hc.UpdateFormations(ctx, app.HerokuAppName, missing); err != nil {
		recordError(db, app, fmt.Errorf("restoring stopped dynos: %w", err))
		return false, err
	}
	recordHistory(db, app, models.ActionWake, ReasonRepair, missing)
	log.Printf("autosleep: %s: was marked awake but had stopped dynos; restored %v", app.HerokuAppName, missing)
	ScheduleNextCheck(app)
	return true, nil
}
