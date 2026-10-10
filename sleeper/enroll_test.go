package sleeper

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"context"
	"testing"
	"time"
)

func TestAutoEnrollReviewApps(t *testing.T) {
	db, fake, sched := setup(t)
	t.Setenv("AUTO_ENROLL", "shop-pr-*, staging")
	t.Setenv("AUTO_ENROLL_IDLE_MINUTES", "15")
	SetPublicURL("https://autosleep.example.com")
	defer SetPublicURL("")

	act := models.Account{Email: "dev@example.com", AccessToken: models.Encrypt("oauth"),
		RefreshToken: models.Encrypt("r"), ExpiredAt: t0.Add(time.Hour)}
	if err := db.Create(&act).Error; err != nil {
		t.Fatal(err)
	}
	// CurrentAccessToken compares against the wall clock.
	db.Model(&act).Update("expired_at", time.Now().Add(time.Hour))
	fake.Apps = []herokuclient.App{{Name: "shop-pr-12"}, {Name: "shop"}, {Name: "staging"}, {Name: "shop-pr-13"}}
	for _, a := range fake.Apps {
		fake.Formations[a.Name] = []herokuclient.Formation{web2}
	}
	newApp(t, db, fake, "shop-pr-13", 0, web2) // already managed
	fake.Apps = append(fake.Apps, herokuclient.App{Name: "shop-pr-14"})
	OptOut(db, "shop-pr-14") // removed by the user earlier

	AutoEnroll(context.Background(), db)

	for _, name := range []string{"shop-pr-12", "staging"} {
		app, err := Load(db, name)
		if err != nil {
			t.Fatalf("%s not enrolled: %v", name, err)
		}
		if app.IdealTime != 900 || app.AccountId == nil || *app.AccountId != act.ID || app.DrainToken == "" {
			t.Errorf("%s enrolled with wrong settings: %+v", name, app)
		}
		if _, ok := sched.Scheduled(name); !ok {
			t.Errorf("%s has no sleep check", name)
		}
	}
	if _, err := Load(db, "shop"); err == nil {
		t.Error("non-matching app was enrolled")
	}
	if _, err := Load(db, "shop-pr-14"); err == nil {
		t.Error("opted-out app was re-enrolled")
	}
	if n := len(fake.Drains["shop-pr-13"]); n != 0 {
		t.Errorf("already-managed app got %d new drains", n)
	}
}
