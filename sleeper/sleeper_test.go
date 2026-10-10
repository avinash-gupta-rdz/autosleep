package sleeper

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"autosleep/schedule"
	"autosleep/testutil"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

var t0 = time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC) // 10:00 UTC, outside default night window

func setup(t *testing.T) (*gorm.DB, *testutil.FakeHeroku, *testutil.Scheduler) {
	db := testutil.NewDB(t)
	fake := (&testutil.FakeHeroku{}).Install(t)
	sched := &testutil.Scheduler{}
	prevChecks, prevNow, prevNight := Checks, Now, Night
	Checks, Now, Night = sched, func() time.Time { return t0 }, schedule.Default
	t.Cleanup(func() { Checks, Now, Night = prevChecks, prevNow, prevNight })
	return db, fake, sched
}

func newApp(t *testing.T, db *gorm.DB, fake *testutil.FakeHeroku, name string, idleFor time.Duration, f ...herokuclient.Formation) *models.Application {
	t.Helper()
	fake.Formations[name] = f
	app := &models.Application{
		HerokuAppName: name, HerokuApiKey: models.Encrypt("key-" + name), CurrentStatus: true,
		RecentActivityAt: t0.Add(-idleFor), IdealTime: 1800, CheckInterval: 600,
	}
	if err := db.Create(app).Error; err != nil {
		t.Fatal(err)
	}
	return app
}

func reload(t *testing.T, db *gorm.DB, name string) *models.Application {
	t.Helper()
	app, err := Load(db, name)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func historyCount(db *gorm.DB, appID uint, action bool) int64 {
	var n int64
	db.Model(&models.AppHistory{}).Where("application_id = ? AND action = ?", appID, action).Count(&n)
	return n
}

var (
	web2   = herokuclient.Formation{Type: "web", Size: "Standard-1X", Quantity: 2}
	worker = herokuclient.Formation{Type: "worker", Size: "Standard-2X", Quantity: 1}
)

func TestIdleAppSleepsAndWakesWithSameFormation(t *testing.T) {
	db, fake, sched := setup(t)
	newApp(t, db, fake, "shop", time.Hour, web2, worker)

	if err := Check(context.Background(), db, "shop"); err != nil {
		t.Fatal(err)
	}
	if got := fake.Running("shop"); got["web"] != 0 || got["worker"] != 0 {
		t.Fatalf("expected all dynos stopped, got %v", got)
	}
	app := reload(t, db, "shop")
	if app.CurrentStatus {
		t.Fatal("expected app marked asleep")
	}
	if app.CurrentConfig["web"]["Quantity"] != "2" || app.CurrentConfig["worker"]["Size"] != "Standard-2X" {
		t.Fatalf("saved config wrong: %v", app.CurrentConfig)
	}
	if n := historyCount(db, app.ID, models.ActionSleep); n != 2 {
		t.Fatalf("expected 2 sleep history rows, got %d", n)
	}

	woke, err := Wake(context.Background(), db, app, ReasonTraffic)
	if err != nil || !woke {
		t.Fatalf("Wake = %v, %v", woke, err)
	}
	if got := fake.Running("shop"); got["web"] != 2 || got["worker"] != 1 {
		t.Fatalf("expected formation restored, got %v", got)
	}
	if !reload(t, db, "shop").CurrentStatus {
		t.Fatal("expected app marked awake")
	}
	if _, ok := sched.Scheduled("shop"); !ok {
		t.Fatal("expected a sleep check to be scheduled after waking")
	}
}

func TestActiveAppIsNotSlept(t *testing.T) {
	db, fake, sched := setup(t)
	newApp(t, db, fake, "busy", 5*time.Minute, web2)

	if err := Check(context.Background(), db, "busy"); err != nil {
		t.Fatal(err)
	}
	if fake.Running("busy")["web"] != 2 {
		t.Fatal("active app must keep running")
	}
	// Idle for 5 of 30 minutes: next check is capped by the 10 minute interval.
	if d, _ := sched.Scheduled("busy"); d != 10*time.Minute {
		t.Fatalf("next check in %v, want 10m", d)
	}
}

func TestNextCheckIsTimedToIdleDeadline(t *testing.T) {
	app := &models.Application{CurrentStatus: true, RecentActivityAt: t0.Add(-25 * time.Minute), IdealTime: 1800, CheckInterval: 600}
	if d := NextCheckIn(app, t0); d != 5*time.Minute {
		t.Fatalf("NextCheckIn = %v, want 5m", d)
	}
	app.RecentActivityAt = t0.Add(-2 * time.Hour)
	if d := NextCheckIn(app, t0); d != MinCheckDelay {
		t.Fatalf("NextCheckIn for overdue app = %v, want %v", d, MinCheckDelay)
	}
}

func TestFormationReadFailureNeverLosesConfig(t *testing.T) {
	db, fake, sched := setup(t)
	app := newApp(t, db, fake, "flaky", time.Hour, web2)
	db.Model(app).Update("current_config", ConfigFrom([]herokuclient.Formation{web2}))
	fake.FailNext["ListFormations"] = errors.New("heroku 503")

	if err := Check(context.Background(), db, "flaky"); err != nil {
		t.Fatal(err)
	}
	app = reload(t, db, "flaky")
	if !app.CurrentStatus {
		t.Fatal("app must stay awake when formation can't be read")
	}
	if app.CurrentConfig["web"]["Quantity"] != "2" {
		t.Fatalf("saved config was clobbered: %v", app.CurrentConfig)
	}
	if app.LastError == "" {
		t.Fatal("expected the failure to be surfaced in last_error")
	}
	if d, _ := sched.Scheduled("flaky"); d != 10*time.Minute {
		t.Fatalf("retry scheduled in %v, want the 10m check interval", d)
	}
}

func TestScaleDownFailureKeepsAppAwake(t *testing.T) {
	db, fake, _ := setup(t)
	newApp(t, db, fake, "stuck", time.Hour, web2)
	fake.FailNext["UpdateFormations"] = errors.New("rate limited")

	if _, err := Sleep(context.Background(), db, reload(t, db, "stuck"), ReasonIdle, false); err == nil {
		t.Fatal("expected error")
	}
	if !reload(t, db, "stuck").CurrentStatus {
		t.Fatal("status must be reverted to awake when scale-down fails")
	}
}

func TestAlreadyScaledToZeroKeepsKnownConfig(t *testing.T) {
	db, fake, _ := setup(t)
	app := newApp(t, db, fake, "zero", time.Hour, herokuclient.Formation{Type: "web", Size: "Basic", Quantity: 0})
	db.Model(app).Update("current_config", ConfigFrom([]herokuclient.Formation{{Type: "web", Size: "Basic", Quantity: 3}}))

	if _, err := Sleep(context.Background(), db, reload(t, db, "zero"), ReasonIdle, false); err != nil {
		t.Fatal(err)
	}
	if q := reload(t, db, "zero").CurrentConfig["web"]["Quantity"]; q != "3" {
		t.Fatalf("known config overwritten with %q", q)
	}
}

func TestSleepAbortsIfTrafficArrived(t *testing.T) {
	db, fake, _ := setup(t)
	newApp(t, db, fake, "race", time.Hour, web2)
	stale := reload(t, db, "race")
	// A request lands between loading the app and putting it to sleep.
	db.Model(&models.Application{}).Where("id = ?", stale.ID).Update("recent_activity_at", t0)

	slept, err := Sleep(context.Background(), db, stale, ReasonIdle, false)
	if err != nil || slept {
		t.Fatalf("Sleep = %v, %v; want no-op", slept, err)
	}
	if fake.Running("race")["web"] != 2 {
		t.Fatal("dynos must not be stopped after fresh traffic")
	}
}

func TestWakeFailureLeavesAppAsleep(t *testing.T) {
	db, fake, _ := setup(t)
	newApp(t, db, fake, "wf", time.Hour, web2)
	if _, err := Sleep(context.Background(), db, reload(t, db, "wf"), ReasonIdle, false); err != nil {
		t.Fatal(err)
	}
	fake.FailNext["UpdateFormations"] = errors.New("boom")
	if _, err := Wake(context.Background(), db, reload(t, db, "wf"), ReasonTraffic); err == nil {
		t.Fatal("expected error")
	}
	if reload(t, db, "wf").CurrentStatus {
		t.Fatal("app must stay marked asleep so the next request retries the wake")
	}
	// Next request succeeds.
	if woke, err := Wake(context.Background(), db, reload(t, db, "wf"), ReasonTraffic); !woke || err != nil {
		t.Fatalf("retry Wake = %v, %v", woke, err)
	}
	if fake.Running("wf")["web"] != 2 {
		t.Fatal("expected formation restored on retry")
	}
}

func TestConcurrentWakesScaleUpOnce(t *testing.T) {
	db, fake, _ := setup(t)
	newApp(t, db, fake, "burst", time.Hour, web2)
	if _, err := Sleep(context.Background(), db, reload(t, db, "burst"), ReasonIdle, false); err != nil {
		t.Fatal(err)
	}
	fake.Calls = nil

	var wg sync.WaitGroup
	var mu sync.Mutex
	woken := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			app, err := Load(db, "burst")
			if err != nil {
				t.Error(err)
				return
			}
			if ok, _ := Wake(context.Background(), db, app, ReasonTraffic); ok {
				mu.Lock()
				woken++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if woken != 1 {
		t.Fatalf("%d goroutines woke the app, want exactly 1", woken)
	}
	updates := 0
	for _, c := range fake.Calls {
		if c == "UpdateFormations" {
			updates++
		}
	}
	if updates != 1 {
		t.Fatalf("formation updated %d times, want 1", updates)
	}
}

func TestWebOnlyLeavesWorkersRunning(t *testing.T) {
	db, fake, _ := setup(t)
	app := newApp(t, db, fake, "webonly", time.Hour, web2, worker)
	db.Model(app).Update("web_only", true)

	if err := Check(context.Background(), db, "webonly"); err != nil {
		t.Fatal(err)
	}
	got := fake.Running("webonly")
	if got["web"] != 0 || got["worker"] != 1 {
		t.Fatalf("web-only sleep should stop web only, got %v", got)
	}
	if _, err := Wake(context.Background(), db, reload(t, db, "webonly"), ReasonTraffic); err != nil {
		t.Fatal(err)
	}
	if got := fake.Running("webonly"); got["web"] != 2 || got["worker"] != 1 {
		t.Fatalf("after wake got %v", got)
	}
}

func TestNightModeOnlySleepsInWindow(t *testing.T) {
	db, fake, sched := setup(t)
	app := newApp(t, db, fake, "nightly", time.Hour, web2)
	db.Model(app).Update("night_mode", true)

	Check(context.Background(), db, "nightly") // 10:00 UTC: daytime
	if fake.Running("nightly")["web"] != 2 {
		t.Fatal("night-mode app slept during the day")
	}
	if _, ok := sched.Scheduled("nightly"); !ok {
		t.Fatal("expected a re-check")
	}

	Now = func() time.Time { return time.Date(2026, 10, 7, 22, 0, 0, 0, time.UTC) }
	db.Model(app).Update("recent_activity_at", Now().Add(-time.Hour))
	Check(context.Background(), db, "nightly")
	if fake.Running("nightly")["web"] != 0 {
		t.Fatal("night-mode app should sleep at night")
	}
}

func TestPausedAppIsNeverSlept(t *testing.T) {
	db, fake, sched := setup(t)
	app := newApp(t, db, fake, "paused", time.Hour, web2)
	db.Model(app).Update("paused", true)

	Check(context.Background(), db, "paused")
	if fake.Running("paused")["web"] != 2 {
		t.Fatal("paused app was slept")
	}
	if _, ok := sched.Scheduled("paused"); ok {
		t.Fatal("paused apps should not keep a check chain alive")
	}
}

func TestEachAppUsesItsOwnToken(t *testing.T) {
	db, fake, _ := setup(t)
	newApp(t, db, fake, "a", time.Hour, web2)
	newApp(t, db, fake, "b", time.Hour, web2)

	var wg sync.WaitGroup
	for _, n := range []string{"a", "b"} {
		wg.Add(1)
		go func(n string) { defer wg.Done(); Check(context.Background(), db, n) }(n)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, tok := range fake.Tokens {
		seen[tok] = true
	}
	if !seen["key-a"] || !seen["key-b"] {
		t.Fatalf("expected per-app tokens, got %v", fake.Tokens)
	}
}

func TestAppWithoutAccountSatisfiesForeignKey(t *testing.T) {
	db, fake, _ := setup(t)
	// Regression: AccountId used to be stored as 0, violating the FK to accounts.
	newApp(t, db, fake, "apikey-only", 0, web2)
	if reload(t, db, "apikey-only").AccountId != nil {
		t.Fatal("expected NULL account_id")
	}
}

func TestNextSleepAt(t *testing.T) {
	app := &models.Application{CurrentStatus: true, RecentActivityAt: t0.Add(-10 * time.Minute), IdealTime: 1800}
	if got := NextSleepAt(app, t0); !got.Equal(t0.Add(20 * time.Minute)) {
		t.Fatalf("NextSleepAt = %v", got)
	}
	app.NightMode = true // window opens 15:30 UTC
	if got := NextSleepAt(app, t0); got.Hour() != 15 || got.Minute() != 30 {
		t.Fatalf("night-mode NextSleepAt = %v, want 15:30", got)
	}
	app.Paused = true
	if !NextSleepAt(app, t0).IsZero() {
		t.Fatal("paused app should have no planned sleep")
	}
}

func TestDeletedHerokuAppIsForgotten(t *testing.T) {
	db, fake, sched := setup(t)
	newApp(t, db, fake, "pr-42", time.Hour, web2)
	fake.FailNext["ListFormations"] = heroku404{}

	if err := Check(context.Background(), db, "pr-42"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(db, "pr-42"); err != gorm.ErrRecordNotFound {
		t.Fatalf("expected app to be forgotten, got %v", err)
	}
	if _, ok := sched.Scheduled("pr-42"); ok {
		t.Fatal("forgotten app should not be rescheduled")
	}
}

func TestScaleDownTimeoutThatSucceededKeepsAppAsleep(t *testing.T) {
	db, fake, _ := setup(t)
	newApp(t, db, fake, "timeout", time.Hour, web2)
	fake.FailAfter["UpdateFormations"] = errors.New("context deadline exceeded")

	slept, err := Sleep(context.Background(), db, reload(t, db, "timeout"), ReasonIdle, false)
	if err != nil || !slept {
		t.Fatalf("Sleep = %v, %v", slept, err)
	}
	if fake.Running("timeout")["web"] != 0 || reload(t, db, "timeout").CurrentStatus {
		t.Fatal("DB must agree with Heroku: dynos are stopped, so the app is asleep")
	}
}

func TestPausedAppCannotBeSleptByStaleCheck(t *testing.T) {
	db, fake, _ := setup(t)
	newApp(t, db, fake, "p", time.Hour, web2)
	stale := reload(t, db, "p")
	db.Model(&models.Application{}).Where("id = ?", stale.ID).Update("paused", true)

	if slept, _ := Sleep(context.Background(), db, stale, ReasonIdle, false); slept {
		t.Fatal("paused app was put to sleep")
	}
	if fake.Running("p")["web"] != 2 {
		t.Fatal("paused app's dynos were stopped")
	}
}

func TestRepairRestoresStoppedDynosOfAwakeApp(t *testing.T) {
	db, fake, _ := setup(t)
	app := newApp(t, db, fake, "broken", 0, web2, worker)
	db.Model(app).Update("current_config", ConfigFrom([]herokuclient.Formation{web2, worker}))
	// A sleep/wake race left the app marked awake with web stopped.
	fake.Formations["broken"] = []herokuclient.Formation{{Type: "web", Size: "Standard-1X", Quantity: 0}, worker}

	fixed, err := Repair(context.Background(), db, reload(t, db, "broken"))
	if err != nil || !fixed {
		t.Fatalf("Repair = %v, %v", fixed, err)
	}
	if got := fake.Running("broken"); got["web"] != 2 || got["worker"] != 1 {
		t.Fatalf("after repair: %v", got)
	}
	if fixed, _ := Repair(context.Background(), db, reload(t, db, "broken")); fixed {
		t.Fatal("healthy app should need no repair")
	}
}

func TestEnrollAdoptsLeftoverDrain(t *testing.T) {
	db, fake, _ := setup(t)
	fake.Formations["again"] = []herokuclient.Formation{web2}
	fake.Drains["again"] = []herokuclient.Drain{{ID: "old", Token: "d.old", URL: "https://as.example.com/drain/again"}}
	app := &models.Application{HerokuAppName: "again", HerokuApiKey: models.Encrypt("k"), CurrentStatus: true, IdealTime: 1800, CheckInterval: 600}
	if err := Enroll(context.Background(), db, app, "https://as.example.com"); err != nil {
		t.Fatal(err)
	}
	if app.DrainId != "old" || app.DrainToken != "d.old" {
		t.Fatalf("did not adopt the existing drain: %q %q", app.DrainId, app.DrainToken)
	}
}
