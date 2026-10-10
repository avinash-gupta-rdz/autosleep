package controllers

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"autosleep/sleeper"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Check is one line of an app's self-diagnosis.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok, warn or fail
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
	// Action names a one-click fix the dashboard can offer ("repair-drain").
	Action string `json:"action,omitempty"`
}

const (
	statusOK   = "ok"
	statusWarn = "warn"
	statusFail = "fail"
)

// quietLogs is how long an app may go without any log batch before the
// drain check warns. Running apps log constantly; a sleeping app may not.
const quietLogs = 15 * time.Minute

// DiagnoseApp checks everything AutoSleep needs to sleep and wake an app and
// explains how to fix what is wrong.
func DiagnoseApp(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	c.JSON(http.StatusOK, gin.H{"list": diagnose(ctx, app, BaseURL(c), sleeper.Now())})
}

func diagnose(ctx context.Context, app *models.Application, base string, now time.Time) []Check {
	var out []Check
	add := func(name, status, detail, fix string) *Check {
		out = append(out, Check{Name: name, Status: status, Detail: detail, Fix: fix})
		return &out[len(out)-1]
	}

	token, err := app.GetToken()
	if err != nil {
		add("Heroku access", statusFail, err.Error(), "Reconnect the Heroku account, or set a new API key in Settings.")
		return out
	}
	hc := herokuclient.New(token)
	formations, err := hc.ListFormations(ctx, app.HerokuAppName)
	switch {
	case herokuclient.IsNotFound(err):
		add("Heroku access", statusFail, "Heroku says this app doesn't exist (deleted or renamed?).", "Remove it from AutoSleep.")
		return out
	case err != nil:
		add("Heroku access", statusFail, "Could not read the app's dynos: "+err.Error(), "Check that the account or API key still has access to this app.")
		return out
	}
	running := 0
	for _, f := range formations {
		running += f.Quantity
	}
	dynos := "dynos"
	if running == 1 {
		dynos = "dyno"
	}
	add("Heroku access", statusOK, fmt.Sprintf("Can read and scale the app (%d %s running now).", running, dynos), "")

	// The drain is how AutoSleep sees traffic and wakes the app.
	want := sleeper.DrainURL(base, app.HerokuAppName)
	drain, derr := herokuclient.Drain{}, error(nil)
	if app.DrainId == "" {
		derr = fmt.Errorf("no drain recorded")
	} else {
		drain, derr = hc.DrainInfo(ctx, app.HerokuAppName, app.DrainId)
	}
	switch {
	case derr != nil:
		ch := add("Log drain", statusFail, "AutoSleep's log drain is missing from the app, so it can't see traffic or wake up.", "Re-install the drain.")
		ch.Action = "repair-drain"
	case drain.URL != want && !strings.HasSuffix(drain.URL, "/drain/"+app.HerokuAppName):
		ch := add("Log drain", statusFail, "The drain points somewhere else: "+drain.URL, "Re-install the drain.")
		ch.Action = "repair-drain"
	case drain.URL != want:
		ch := add("Log drain", statusWarn, "The drain points at "+drain.URL+", not this address ("+want+").",
			"If AutoSleep moved to a new domain, re-install the drain. If you open the dashboard through a different hostname, set SELF_HOST instead.")
		ch.Action = "repair-drain"
	default:
		add("Log drain", statusOK, "Installed and pointing at this AutoSleep.", "")
	}

	switch {
	case app.LastDrainAt == nil && now.Sub(app.CreatedAt) < 5*time.Minute:
		add("Logs arriving", statusWarn, "No logs received yet. Heroku usually starts sending within a minute or two.", "Open the app in a browser to generate some traffic.")
	case app.LastDrainAt == nil:
		add("Logs arriving", statusFail, "Heroku has never delivered logs for this app, so AutoSleep can't wake it.",
			"Make sure this AutoSleep is reachable from the internet over HTTPS at "+base+", then re-install the drain.").Action = "repair-drain"
	case now.Sub(*app.LastDrainAt) > quietLogs && app.CurrentStatus:
		add("Logs arriving", statusWarn, "Last logs arrived "+ago(now, *app.LastDrainAt)+" although the app is awake.", "Visit the app; if logs still don't arrive, re-install the drain.").Action = "repair-drain"
	default:
		add("Logs arriving", statusOK, "Last logs arrived "+ago(now, *app.LastDrainAt)+".", "")
	}

	if !app.CurrentStatus && len(sleeper.FormationsFrom(app.CurrentConfig)) == 0 {
		add("Saved formation", statusFail, "The app is asleep but no dyno formation is saved, so waking it starts nothing.", "Scale it manually with `heroku ps:scale web=1 -a "+app.HerokuAppName+"`, then press Wake.")
	} else {
		add("Saved formation", statusOK, "Will restore: "+formationText(app.CurrentConfig)+".", "")
	}

	if app.LastError != "" {
		when := ""
		if app.LastErrorAt != nil {
			when = " (" + ago(now, *app.LastErrorAt) + ")"
		}
		add("Last error", statusWarn, app.LastError+when, "It clears after the next successful sleep or wake.")
	}

	switch {
	case app.Paused:
		add("Mode", statusWarn, "Paused: AutoSleep won't put this app to sleep.", "Resume it to save money again.")
	case app.ManualMode:
		add("Mode", statusWarn, "Manual wake: visitors won't wake it; use Wake in the dashboard.", "")
	case !app.WakePage:
		add("Waking page", statusWarn, "Visitors see Heroku's error page while the app wakes.", "Turn on the waking page in Settings.")
	}
	return out
}

func formationText(cfg models.JSONB) string {
	fs := sleeper.FormationsFrom(cfg)
	if len(fs) == 0 {
		return "nothing yet (captured at the next sleep)"
	}
	parts := make([]string, 0, len(fs))
	for _, f := range fs {
		parts = append(parts, fmt.Sprintf("%s=%d:%s", f.Type, f.Quantity, f.Size))
	}
	return strings.Join(parts, ", ")
}

func ago(now, t time.Time) string {
	d := now.Sub(t).Round(time.Minute)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%.0f h ago", d.Hours())
	default:
		return fmt.Sprintf("%.0f days ago", d.Hours()/24)
	}
}

// RepairDrain re-installs an app's log drain.
func RepairDrain(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	if err := sleeper.RepairDrain(ctx, models.DB, app, BaseURL(c)); err != nil {
		jsonError(c, http.StatusBadGateway, err.Error())
		return
	}
	respondApp(c, app.HerokuAppName)
}
