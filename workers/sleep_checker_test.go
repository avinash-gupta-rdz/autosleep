package workers

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"autosleep/sleeper"
	"autosleep/testutil"
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"
	heroku "github.com/heroku/heroku-go/v5"
)

type fakeQueue struct {
	testutil.Scheduler
	now []string
}

func (f *fakeQueue) CheckNow(app string) error { f.now = append(f.now, app); return nil }

func TestReconcileReschedulesAndForcesOverdue(t *testing.T) {
	db := testutil.NewDB(t)
	fake := (&testutil.FakeHeroku{}).Install(t)
	t0 := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	prev := sleeper.Now
	sleeper.Now = func() time.Time { return t0 }
	defer func() { sleeper.Now = prev }()

	mk := func(name string, awake bool, idle time.Duration, mut func(*models.Application)) {
		app := models.Application{HerokuAppName: name, HerokuApiKey: models.Encrypt("k"), CurrentStatus: awake,
			RecentActivityAt: t0.Add(-idle), IdealTime: 1800, CheckInterval: 600}
		if mut != nil {
			mut(&app)
		}
		if err := db.Create(&app).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk("fresh", true, time.Minute, nil)
	mk("overdue", true, 3*time.Hour, nil)
	mk("asleep", false, 3*time.Hour, nil)
	mk("paused", true, 3*time.Hour, func(a *models.Application) { a.Paused = true })
	mk("legacy", false, 0, func(a *models.Application) { a.DrainId = "drain-legacy-1" })
	fake.Drains["legacy"] = []herokuclient.Drain{{ID: "drain-legacy-1", Token: "d.legacy"}}

	q := &fakeQueue{}
	Reconcile(context.Background(), db, q)

	if _, ok := q.Scheduled("fresh"); !ok {
		t.Error("awake app should get a check scheduled")
	}
	if len(q.now) != 1 || q.now[0] != "overdue" {
		t.Errorf("forced checks = %v, want [overdue]", q.now)
	}
	for _, n := range []string{"asleep", "paused"} {
		if _, ok := q.Scheduled(n); ok {
			t.Errorf("%s should not be scheduled", n)
		}
	}
	var legacy models.Application
	db.Where("heroku_app_name = ?", "legacy").First(&legacy)
	if legacy.DrainToken != "d.legacy" {
		t.Errorf("drain token not backfilled: %q", legacy.DrainToken)
	}
}

func TestQueueDeduplicatesScheduledChecks(t *testing.T) {
	mr := miniredis.RunT(t)
	pool := &redis.Pool{Dial: func() (redis.Conn, error) { return redis.Dial("tcp", mr.Addr()) }}
	q := Queue{Pool: pool}

	for i := 0; i < 3; i++ {
		if err := q.ScheduleCheck("shop", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.ScheduleCheck("other", time.Minute); err != nil {
		t.Fatal(err)
	}
	members, err := mr.ZMembers("auto_ideal:scheduled")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("scheduled %d jobs, want 2 (one per app)", len(members))
	}
	if err := q.CheckNow("shop"); err != nil {
		t.Fatal(err)
	}
	if l, _ := mr.List("auto_ideal:jobs:sleep_checker"); len(l) != 1 {
		t.Fatalf("CheckNow should enqueue immediately, queue = %v", l)
	}
}

type notFound struct{}

func (notFound) Error() string { return "Couldn't find that app." }
func (notFound) As(target interface{}) bool {
	if e, ok := target.(*heroku.Error); ok {
		*e = heroku.Error{StatusCode: 404}
		return true
	}
	return false
}

func TestReconcileForgetsDeletedSleepingApps(t *testing.T) {
	db := testutil.NewDB(t)
	fake := (&testutil.FakeHeroku{}).Install(t)
	app := models.Application{HerokuAppName: "pr-9", HerokuApiKey: models.Encrypt("k"), CurrentStatus: false, IdealTime: 1800, CheckInterval: 600}
	db.Create(&app)
	// Pick a time when this app is due for its hourly probe.
	var at time.Time
	for i := int64(0); i < 12; i++ {
		at = time.Unix(i*300, 0)
		if probeDue(app.ID, at) {
			break
		}
	}
	prev := sleeper.Now
	sleeper.Now = func() time.Time { return at }
	defer func() { sleeper.Now = prev }()

	fake.FailNext["ListFormations"] = notFound{}
	Reconcile(context.Background(), db, &fakeQueue{})
	if err := db.Where("heroku_app_name = ?", "pr-9").First(&models.Application{}).Error; err == nil {
		t.Fatal("deleted sleeping app was not forgotten")
	}
}
