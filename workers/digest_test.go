package workers

import (
	"autosleep/models"
	"autosleep/testutil"
	"strings"
	"testing"
	"time"
)

func TestDigestText(t *testing.T) {
	db := testutil.NewDB(t)
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	app := models.Application{HerokuAppName: "shop", IdealTime: 1800, CheckInterval: 600, LastError: "token revoked"}
	db.Create(&app)
	db.Model(&app).Update("created_at", now.AddDate(0, 0, -30))
	// Asleep for two of the last seven days on a Standard-1X.
	db.Create(&models.AppHistory{ApplicationId: app.ID, Action: models.ActionSleep, DynoName: "web", DynoSize: "Standard-1X", Quantity: 1, PerformedAt: now.AddDate(0, 0, -3)})
	db.Create(&models.AppHistory{ApplicationId: app.ID, Action: models.ActionWake, DynoName: "web", DynoSize: "Standard-1X", Quantity: 1, PerformedAt: now.AddDate(0, 0, -1)})

	text := DigestText(db, now)
	for _, want := range []string{"saved *$", "1 app ", "Top savers: shop $", "Needs attention: shop"} {
		if !strings.Contains(text, want) {
			t.Errorf("digest missing %q:\n%s", want, text)
		}
	}
}
