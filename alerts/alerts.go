// Package alerts posts notifications to a chat webhook (ALERT_WEBHOOK_URL):
// failed sleeps and wakes, recoveries, apps forgotten because they were
// deleted on Heroku, and a weekly savings digest. The payload works with
// Slack and Discord incoming webhooks and is easy to read for anything else
// (e.g. a Zapier or Make hook that forwards it by email).
package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// URL is the webhook to post to. Empty disables alerts.
var URL = os.Getenv("ALERT_WEBHOOK_URL")

// Enabled reports whether a webhook is configured.
func Enabled() bool { return URL != "" }

// Payload is what is posted. Slack reads "text", Discord reads "content".
type Payload struct {
	Text    string `json:"text"`
	Content string `json:"content"`
	Event   string `json:"event"`
	App     string `json:"app,omitempty"`
}

// post sends a payload. It is a variable so tests can capture alerts.
var post = func(ctx context.Context, p Payload) error {
	body, _ := json.Marshal(p)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook answered %s", resp.Status)
	}
	return nil
}

// Send posts a message now and reports delivery errors.
func Send(ctx context.Context, event, app, text string) error {
	if !Enabled() {
		return fmt.Errorf("ALERT_WEBHOOK_URL is not set")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return post(ctx, Payload{Text: text, Content: text, Event: event, App: app})
}

// notify sends in the background so alerts never slow down sleeping and
// waking.
func notify(event, app, text string) {
	if !Enabled() {
		return
	}
	go func() {
		if err := Send(context.Background(), event, app, text); err != nil {
			log.Printf("alerts: %v", err)
		}
	}()
}

// errorEvery limits error alerts to one per app per hour, so a failing
// token doesn't flood the channel.
const errorEvery = time.Hour

var (
	mu       sync.Mutex
	lastSent = map[string]time.Time{}
)

// AppError reports a failed sleep, wake or other action on an app.
func AppError(app, msg string, now time.Time) {
	if !Enabled() {
		return
	}
	mu.Lock()
	last, ok := lastSent[app]
	due := !ok || now.Sub(last) >= errorEvery
	if due {
		lastSent[app] = now
	}
	mu.Unlock()
	if due {
		notify("app_error", app, fmt.Sprintf(":warning: AutoSleep: *%s*: %s", app, msg))
	}
}

// AppRecovered reports that an app works again, if an error was alerted.
func AppRecovered(app string) {
	mu.Lock()
	_, alerted := lastSent[app]
	delete(lastSent, app)
	mu.Unlock()
	if alerted {
		notify("app_recovered", app, fmt.Sprintf(":white_check_mark: AutoSleep: *%s* is working again.", app))
	}
}

// AppForgotten reports an app removed because it no longer exists on
// Heroku (e.g. a closed review app).
func AppForgotten(app string) {
	notify("app_forgotten", app, fmt.Sprintf("AutoSleep: *%s* was deleted on Heroku, so it is no longer managed.", app))
}
