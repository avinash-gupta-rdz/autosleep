package activity

import (
	"strconv"
	"testing"
)

func frame(lines ...string) []byte {
	var b []byte
	for _, l := range lines {
		b = append(b, []byte(strconv.Itoa(len(l))+" "+l)...)
	}
	return b
}

const (
	routerOK   = `<158>1 2026-10-07T06:45:26+00:00 host heroku router - at=info method=GET path="/" host=myapp.herokuapp.com request_id=1 fwd="1.2.3.4" dyno=web.1 connect=0ms service=5ms status=200 bytes=100`
	routerH14  = `<158>1 2026-10-07T06:45:26+00:00 host heroku router - at=error code=H14 desc="No web processes running" method=GET path="/" host=myapp.herokuapp.com`
	acme       = `<158>1 2026-10-07T06:45:26+00:00 host heroku router - at=info method=GET path="/.well-known/acme-challenge/x" host=myapp.herokuapp.com status=200`
	appLog     = `<190>1 2026-10-07T06:45:26+00:00 host app web.1 - Mounted router at /api`
	scaleEvent = `<45>1 2026-10-07T06:45:26+00:00 host heroku api - Scaled to web@0:Standard-1X by user autosleep`
)

func TestHasTraffic(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want bool
	}{
		{"router request", frame(routerOK), true},
		{"router H14 while asleep", frame(routerH14), true},
		{"app log mentioning router", frame(appLog), false},
		{"platform event", frame(scaleEvent), false},
		{"acme challenge only", frame(acme), false},
		{"acme then real request", frame(acme, routerOK), true},
		{"router after many app logs", frame(appLog, appLog, appLog, scaleEvent, routerOK), true},
		{"legacy doc sample", []byte("83 <40>1 2012-11-30T06:45:29+00:00 host router www.3 - State changed from starting to up\n"), true},
		{"unframed lines", []byte(appLog + "\n" + routerOK + "\n"), true},
		{"empty", nil, false},
		{"garbage", []byte("hello world"), false},
	}
	for _, c := range cases {
		if got := HasTraffic(c.body); got != c.want {
			t.Errorf("%s: HasTraffic = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestParseFrames(t *testing.T) {
	msgs := Parse(frame(routerOK, appLog))
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].App != "heroku" || msgs[0].Proc != "router" {
		t.Errorf("first message parsed as %+v", msgs[0])
	}
	if msgs[1].App != "app" || msgs[1].Proc != "web.1" || msgs[1].Text != "Mounted router at /api" {
		t.Errorf("second message parsed as %+v", msgs[1])
	}
}

func TestAnalyzeDetectsH14(t *testing.T) {
	if s := Analyze(frame(routerOK)); !s.Traffic || s.NoDynos {
		t.Errorf("normal request: %+v", s)
	}
	if s := Analyze(frame(routerOK, routerH14)); !s.Traffic || !s.NoDynos {
		t.Errorf("H14 batch: %+v", s)
	}
}
