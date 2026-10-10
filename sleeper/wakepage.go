package sleeper

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"context"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
)

// errorPageVar is Heroku's config var for the page shown on router errors
// such as H14 "No web processes running" — i.e. while an app sleeps.
const errorPageVar = "ERROR_PAGE_URL"

func wakePageURL(base, app string) string { return base + "/waking/" + app }

// SetWakePage points the app's ERROR_PAGE_URL at our waking page, or
// removes it. It refuses to overwrite an error page the user set up
// themselves. Changing a config var restarts the app's dynos.
func SetWakePage(ctx context.Context, app *models.Application, base string, enable bool) error {
	token, err := app.GetToken()
	if err != nil {
		return err
	}
	hc := herokuclient.New(token)
	current, err := hc.GetConfigVar(ctx, app.HerokuAppName, errorPageVar)
	if err != nil {
		return err
	}
	ours := strings.HasSuffix(current, "/waking/"+app.HerokuAppName)
	if enable {
		if current != "" && !ours {
			return fmt.Errorf("%s already has %s=%s; remove it first to use the AutoSleep waking page", app.HerokuAppName, errorPageVar, current)
		}
		url := wakePageURL(base, app.HerokuAppName)
		if current == url {
			return nil
		}
		return hc.SetConfigVar(ctx, app.HerokuAppName, errorPageVar, &url)
	}
	if !ours {
		return nil // never remove a page we didn't set
	}
	return hc.SetConfigVar(ctx, app.HerokuAppName, errorPageVar, nil)
}

// WakePageByDefault reports whether newly enrolled apps get the waking page.
// WAKE_PAGE_DEFAULT=false turns it off, e.g. for apps whose restart on
// enrolment would be disruptive.
func WakePageByDefault() bool {
	return !strings.EqualFold(os.Getenv("WAKE_PAGE_DEFAULT"), "false")
}

// EnableWakePageOnEnroll turns on the waking page for a freshly enrolled
// app. Failure is not fatal: the app is managed either way, and the reason
// is returned for display.
func EnableWakePageOnEnroll(ctx context.Context, db *gorm.DB, app *models.Application, base string) error {
	if err := SetWakePage(ctx, app, base, true); err != nil {
		return err
	}
	app.WakePage = true
	return db.Model(&models.Application{}).Where("id = ?", app.ID).Update("wake_page", true).Error
}
