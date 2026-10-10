package alerts

import (
	"context"
	"strings"
	"testing"
	"time"
)

func capture(t *testing.T) chan Payload {
	ch := make(chan Payload, 10)
	prevURL, prevPost := URL, post
	URL = "https://hooks.example.com/x"
	post = func(_ context.Context, p Payload) error { ch <- p; return nil }
	t.Cleanup(func() { URL, post = prevURL, prevPost; lastSent = map[string]time.Time{} })
	return ch
}

func next(t *testing.T, ch chan Payload) (Payload, bool) {
	select {
	case p := <-ch:
		return p, true
	case <-time.After(200 * time.Millisecond):
		return Payload{}, false
	}
}

func TestErrorAlertsAreThrottledAndRecoveryFollows(t *testing.T) {
	ch := capture(t)
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)

	AppRecovered("shop") // nothing was alerted, so no recovery message
	if _, ok := next(t, ch); ok {
		t.Fatal("recovery sent without a prior error")
	}
	AppError("shop", "scaling up: 401", now)
	p, ok := next(t, ch)
	if !ok || p.App != "shop" || p.Event != "app_error" || !strings.Contains(p.Text, "401") || p.Content != p.Text {
		t.Fatalf("error alert = %+v", p)
	}
	AppError("shop", "again", now.Add(10*time.Minute))
	if _, ok := next(t, ch); ok {
		t.Fatal("second error within the hour was not throttled")
	}
	AppRecovered("shop")
	if p, ok := next(t, ch); !ok || p.Event != "app_recovered" {
		t.Fatalf("recovery = %+v %v", p, ok)
	}
	AppError("shop", "later", now.Add(20*time.Minute)) // recovery resets the throttle
	if _, ok := next(t, ch); !ok {
		t.Fatal("new error after recovery not sent")
	}
}

func TestDisabledWithoutURL(t *testing.T) {
	if err := Send(context.Background(), "test", "", "x"); err == nil && !Enabled() {
		t.Fatal("Send without a URL should fail")
	}
}
