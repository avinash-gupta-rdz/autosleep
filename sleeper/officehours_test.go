package sleeper

import (
	"context"
	"testing"
	"time"
)

// t0 is Wednesday 2026-10-07 10:00 UTC.
func TestOfficeHoursKeepAppAwake(t *testing.T) {
	db, fake, sched := setup(t)
	app := newApp(t, db, fake, "office", 2*time.Hour, web2)
	db.Model(app).Updates(map[string]interface{}{"awake_days": "mon-fri", "awake_from": "09:00", "awake_to": "18:00"})
	app = reload(t, db, "office")

	// Idle for two hours, but inside office hours: stays up.
	if err := Check(context.Background(), db, "office"); err != nil {
		t.Fatal(err)
	}
	if fake.Running("office")["web"] != 2 {
		t.Fatal("slept during office hours")
	}
	if in, _ := sched.Scheduled("office"); in != 10*time.Minute {
		t.Errorf("next check in %v, want the check interval", in)
	}
	if at := NextSleepAt(app, t0); !at.Equal(time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC)) {
		t.Errorf("NextSleepAt = %v, want 18:00", at)
	}

	// After hours it sleeps as usual.
	Now = func() time.Time { return time.Date(2026, 10, 7, 18, 5, 0, 0, time.UTC) }
	if err := Check(context.Background(), db, "office"); err != nil {
		t.Fatal(err)
	}
	if fake.Running("office")["web"] != 0 {
		t.Fatal("did not sleep after office hours")
	}
	app = reload(t, db, "office")

	// Thursday 08:00: still outside, nothing happens.
	if woke, _ := WakeForOfficeHours(context.Background(), db, app, time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)); woke {
		t.Fatal("woke before office hours")
	}
	// Thursday 09:02: woken for the day.
	thu := time.Date(2026, 10, 8, 9, 2, 0, 0, time.UTC)
	Now = func() time.Time { return thu }
	if woke, err := WakeForOfficeHours(context.Background(), db, app, thu); !woke || err != nil {
		t.Fatalf("not woken at start of office hours: %v %v", woke, err)
	}
	if fake.Running("office")["web"] != 2 {
		t.Fatalf("formation not restored: %v", fake.Running("office"))
	}

	// Put to sleep by hand at 11:00: it is not woken again that day.
	Now = func() time.Time { return thu.Add(2 * time.Hour) }
	app = reload(t, db, "office")
	if ok, err := Sleep(context.Background(), db, app, ReasonManual, true); !ok || err != nil {
		t.Fatalf("manual sleep: %v %v", ok, err)
	}
	app = reload(t, db, "office")
	if woke, _ := WakeForOfficeHours(context.Background(), db, app, thu.Add(3*time.Hour)); woke {
		t.Fatal("woke an app that was put to sleep by hand during office hours")
	}
}
