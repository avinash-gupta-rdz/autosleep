package controllers

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"autosleep/sleeper"
	"autosleep/testutil"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var t0 = time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)

type env struct {
	t     *testing.T
	db    *gorm.DB
	fake  *testutil.FakeHeroku
	sched *testutil.Scheduler
	r     *gin.Engine
}

func setup(t *testing.T) *env {
	gin.SetMode(gin.TestMode)
	e := &env{t: t, db: testutil.NewDB(t), fake: (&testutil.FakeHeroku{}).Install(t), sched: &testutil.Scheduler{}}
	prevChecks, prevNow, prevBG := sleeper.Checks, sleeper.Now, WakeInBackground
	sleeper.Checks, sleeper.Now, WakeInBackground = e.sched, func() time.Time { return t0 }, false
	t.Cleanup(func() { sleeper.Checks, sleeper.Now, WakeInBackground = prevChecks, prevNow, prevBG })

	r := gin.New()
	r.POST("/drain/:app_id", ProcessDrain)
	r.GET("/waking/:app_id", WakingPage)
	r.GET("/app", FindApplications)
	r.POST("/app", CreateApp)
	r.GET("/app/:app_id", FindApp)
	r.PATCH("/app/:app_id", UpdateApp)
	r.DELETE("/app/:app_id", DeleteApp)
	r.POST("/app/:app_id/wake", WakeApp)
	r.POST("/app/:app_id/sleep", SleepApp)
	r.POST("/app/:app_id/pause", PauseApp)
	r.POST("/app/:app_id/resume", ResumeApp)
	r.GET("/app/:app_id/history", AppHistory)
	r.GET("/app/:app_id/savings", AppSavings)
	r.GET("/savings", AllSavings)
	e.r = r
	return e
}

func (e *env) do(method, path string, body interface{}, headers ...string) (int, map[string]interface{}) {
	e.t.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case nil:
	case string:
		buf.WriteString(b)
	default:
		json.NewEncoder(&buf).Encode(b)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Host = "autosleep.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	out := map[string]interface{}{}
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *env) app(name string) *models.Application {
	e.t.Helper()
	app, err := sleeper.Load(e.db, name)
	if err != nil {
		e.t.Fatal(err)
	}
	return app
}

func (e *env) create(name string, f ...herokuclient.Formation) {
	e.t.Helper()
	e.fake.Formations[name] = f
	code, body := e.do("POST", "/app", map[string]interface{}{"heroku_app_name": name, "heroku_api_key": "secret-key"})
	if code != 200 {
		e.t.Fatalf("create %s: %d %v", name, code, body)
	}
}

func routerLine(path string) string {
	l := `<158>1 2026-10-07T06:45:26+00:00 host heroku router - at=info method=GET path="` + path + `" status=200`
	return strconv.Itoa(len(l)) + " " + l
}

var web1 = herokuclient.Formation{Type: "web", Size: "Standard-1X", Quantity: 1}

func TestCreateAppInstallsDrainAndHidesSecrets(t *testing.T) {
	e := setup(t)
	e.create("shop", web1)

	app := e.app("shop")
	if app.DrainId == "" || app.DrainToken == "" {
		t.Fatalf("drain not stored: %+v", app)
	}
	if got := e.fake.Drains["shop"][0].URL; got != "https://autosleep.example.com/drain/shop" {
		t.Fatalf("drain URL = %q", got)
	}
	if _, ok := e.sched.Scheduled("shop"); !ok {
		t.Fatal("no sleep check scheduled for new app")
	}
	_, body := e.do("GET", "/app/shop", nil)
	raw, _ := json.Marshal(body)
	for _, secret := range []string{"secret-key", app.DrainToken, "heroku_api_key", "HerokuApiKey", "access_token"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("response leaks %q: %s", secret, raw)
		}
	}
	if data := body["data"].(map[string]interface{}); data["heroku_app_name"] != "shop" || data["status"] != "awake" {
		t.Errorf("unexpected view: %v", data)
	}
}

func TestCreateAppRejectsBadInput(t *testing.T) {
	e := setup(t)
	e.create("dup", web1)
	cases := []struct {
		body map[string]interface{}
		code int
	}{
		{map[string]interface{}{"heroku_app_name": "x"}, 400},
		{map[string]interface{}{"heroku_app_name": "x", "heroku_api_key": "k", "ideal_time": 5}, 400},
		{map[string]interface{}{"heroku_app_name": "dup", "heroku_api_key": "k"}, 409},
		{map[string]interface{}{"heroku_app_name": "x", "account_id": 99}, 400},
	}
	for _, c := range cases {
		if code, body := e.do("POST", "/app", c.body); code != c.code {
			t.Errorf("POST %v = %d %v, want %d", c.body, code, body, c.code)
		}
	}
}

func TestCreateAppFailsCleanlyWhenHerokuRejects(t *testing.T) {
	e := setup(t)
	e.fake.FailNext["CreateDrain"] = errors.New("forbidden")
	code, _ := e.do("POST", "/app", map[string]interface{}{"heroku_app_name": "nope", "heroku_api_key": "k"})
	if code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", code)
	}
	var n int64
	e.db.Model(&models.Application{}).Count(&n)
	if n != 0 {
		t.Fatal("app saved despite drain failure")
	}
}

func TestDrainTrafficWakesSleepingApp(t *testing.T) {
	e := setup(t)
	e.create("shop", herokuclient.Formation{Type: "web", Size: "Standard-1X", Quantity: 3})
	if code, b := e.do("POST", "/app/shop/sleep", nil); code != 200 {
		t.Fatalf("sleep: %d %v", code, b)
	}
	if e.fake.Running("shop")["web"] != 0 {
		t.Fatal("not asleep")
	}
	token := e.app("shop").DrainToken

	// App logs alone don't wake it.
	appLog := `<190>1 2026-10-07T06:45:26+00:00 host app web.1 - router mounted`
	e.do("POST", "/drain/shop", strconv.Itoa(len(appLog))+" "+appLog, "Logplex-Drain-Token", token)
	if e.fake.Running("shop")["web"] != 0 {
		t.Fatal("woke on a non-router log")
	}

	code, _ := e.do("POST", "/drain/shop", routerLine("/"), "Logplex-Drain-Token", token)
	if code != 200 {
		t.Fatalf("drain = %d", code)
	}
	if e.fake.Running("shop")["web"] != 3 {
		t.Fatalf("expected 3 web dynos restored, got %v", e.fake.Running("shop"))
	}
}

func TestDrainRejectsWrongToken(t *testing.T) {
	e := setup(t)
	e.create("shop", web1)
	e.do("POST", "/app/shop/sleep", nil)
	code, _ := e.do("POST", "/drain/shop", routerLine("/"), "Logplex-Drain-Token", "d.forged")
	if code != http.StatusForbidden {
		t.Fatalf("forged drain = %d, want 403", code)
	}
	if e.fake.Running("shop")["web"] != 0 {
		t.Fatal("forged drain woke the app")
	}
}

func TestDrainRespectsManualModeAndPause(t *testing.T) {
	e := setup(t)
	e.create("manual", web1)
	e.do("PATCH", "/app/manual", map[string]interface{}{"manual_mode": true})
	e.do("POST", "/app/manual/sleep", nil)
	e.do("POST", "/drain/manual", routerLine("/"), "Logplex-Drain-Token", e.app("manual").DrainToken)
	if e.fake.Running("manual")["web"] != 0 {
		t.Fatal("manual-mode app woke on traffic")
	}
	if code, _ := e.do("POST", "/app/manual/wake", nil); code != 200 || e.fake.Running("manual")["web"] != 1 {
		t.Fatal("manual wake endpoint did not wake the app")
	}
}

func TestDeleteWakesSleepingAppFirst(t *testing.T) {
	e := setup(t)
	e.create("bye", herokuclient.Formation{Type: "web", Size: "Basic", Quantity: 2})
	e.do("POST", "/app/bye/sleep", nil)
	if code, b := e.do("DELETE", "/app/bye", nil); code != 200 {
		t.Fatalf("delete: %d %v", code, b)
	}
	if e.fake.Running("bye")["web"] != 2 {
		t.Fatal("deleted app left scaled to zero")
	}
	if len(e.fake.Drains["bye"]) != 0 {
		t.Fatal("drain not removed")
	}
	var apps, hist int64
	e.db.Unscoped().Model(&models.Application{}).Count(&apps)
	e.db.Unscoped().Model(&models.AppHistory{}).Count(&hist)
	if apps != 0 || hist != 0 {
		t.Fatalf("leftover rows: apps=%d history=%d", apps, hist)
	}
}

func TestDeleteRefusesWhenWakeFailsUnlessForced(t *testing.T) {
	e := setup(t)
	e.create("stuck", web1)
	e.do("POST", "/app/stuck/sleep", nil)
	e.fake.FailNext["UpdateFormations"] = errors.New("heroku down")
	if code, _ := e.do("DELETE", "/app/stuck", nil); code != http.StatusBadGateway {
		t.Fatalf("delete = %d, want 502", code)
	}
	e.fake.FailNext["UpdateFormations"] = errors.New("heroku down")
	if code, _ := e.do("DELETE", "/app/stuck?force=true", nil); code != 200 {
		t.Fatalf("forced delete = %d", code)
	}
}

func TestPauseWakesAndResumeRestartsClock(t *testing.T) {
	e := setup(t)
	e.create("p", web1)
	e.do("POST", "/app/p/sleep", nil)
	code, body := e.do("POST", "/app/p/pause", nil)
	if code != 200 || body["data"].(map[string]interface{})["status"] != "paused" {
		t.Fatalf("pause: %d %v", code, body)
	}
	if e.fake.Running("p")["web"] != 1 {
		t.Fatal("pausing a sleeping app should wake it")
	}
	if code, _ := e.do("POST", "/app/p/sleep", nil); code != http.StatusConflict {
		t.Fatalf("sleeping a paused app = %d, want 409", code)
	}
	e.sched.Checks = nil
	_, body = e.do("POST", "/app/p/resume", nil)
	if body["data"].(map[string]interface{})["status"] != "awake" {
		t.Fatalf("resume: %v", body)
	}
	if _, ok := e.sched.Scheduled("p"); !ok {
		t.Fatal("resume should schedule a sleep check")
	}
}

func TestPatchValidatesAndUpdates(t *testing.T) {
	e := setup(t)
	e.create("cfg", web1)
	if code, _ := e.do("PATCH", "/app/cfg", map[string]interface{}{"check_interval": 1}); code != 400 {
		t.Fatalf("tiny interval accepted: %d", code)
	}
	code, body := e.do("PATCH", "/app/cfg", map[string]interface{}{"ideal_time": 900, "web_only": true})
	if code != 200 {
		t.Fatalf("patch: %d %v", code, body)
	}
	app := e.app("cfg")
	if app.IdealTime != 900 || !app.WebOnly || app.CheckInterval != 600 {
		t.Fatalf("patch applied wrongly: %+v", app)
	}
}

func TestWakePageConfigVar(t *testing.T) {
	e := setup(t)
	e.create("wp", web1)
	if code, b := e.do("PATCH", "/app/wp", map[string]interface{}{"wake_page": true}); code != 200 {
		t.Fatalf("enable: %d %v", code, b)
	}
	if got := e.fake.ConfigVars["wp"]["ERROR_PAGE_URL"]; got != "https://autosleep.example.com/waking/wp" {
		t.Fatalf("ERROR_PAGE_URL = %q", got)
	}
	e.do("PATCH", "/app/wp", map[string]interface{}{"wake_page": false})
	if _, ok := e.fake.ConfigVars["wp"]["ERROR_PAGE_URL"]; ok {
		t.Fatal("ERROR_PAGE_URL not removed")
	}

	// Never clobber a user's own error page.
	e.create("custom", web1)
	e.fake.ConfigVars["custom"] = map[string]string{"ERROR_PAGE_URL": "https://cdn.example.com/oops.html"}
	if code, _ := e.do("PATCH", "/app/custom", map[string]interface{}{"wake_page": true}); code != http.StatusBadGateway {
		t.Fatalf("overwrote custom error page: %d", code)
	}
	if e.fake.ConfigVars["custom"]["ERROR_PAGE_URL"] != "https://cdn.example.com/oops.html" {
		t.Fatal("custom error page changed")
	}

	req := httptest.NewRequest("GET", "/waking/%3Cscript%3E", nil)
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>is") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatalf("waking page not escaped: %d", w.Code)
	}
}

func TestSavingsEndpoints(t *testing.T) {
	e := setup(t)
	e.create("saver", herokuclient.Formation{Type: "web", Size: "Standard-2X", Quantity: 2})
	// Enrolled 2 days ago, slept for 10 hours yesterday.
	e.db.Model(&models.Application{}).Where("heroku_app_name = ?", "saver").Update("created_at", t0.Add(-48*time.Hour))
	sleeper.Now = func() time.Time { return t0.Add(-30 * time.Hour) }
	e.do("POST", "/app/saver/sleep", nil)
	sleeper.Now = func() time.Time { return t0.Add(-20 * time.Hour) }
	e.do("POST", "/app/saver/wake", nil)
	sleeper.Now = func() time.Time { return t0 }

	code, body := e.do("GET", "/app/saver/savings?days=7", nil)
	if code != 200 {
		t.Fatalf("savings: %d %v", code, body)
	}
	data := body["data"].(map[string]interface{})
	if data["slept_dyno_hours"].(float64) != 20 || data["saved_usd"].(float64) != 1.4 {
		t.Fatalf("report = %v", data)
	}
	_, body = e.do("GET", "/savings", nil)
	sum := body["data"].(map[string]interface{})
	if sum["saved_usd"].(float64) != 1.4 || sum["awake"].(float64) != 1 {
		t.Fatalf("summary = %v", sum)
	}
	_, body = e.do("GET", "/app/saver/history", nil)
	if n := len(body["list"].([]interface{})); n != 2 {
		t.Fatalf("history has %d events, want 2", n)
	}
}

func TestDrainH14OnAwakeAppTriggersRepair(t *testing.T) {
	e := setup(t)
	e.create("heal", herokuclient.Formation{Type: "web", Size: "Standard-1X", Quantity: 2})
	// Marked awake, but the dynos are stopped (lost race / interrupted wake).
	e.fake.Formations["heal"][0].Quantity = 0
	h14 := `<158>1 2026-10-07T06:45:26+00:00 host heroku router - at=error code=H14 desc="No web processes running" method=GET path="/"`
	e.do("POST", "/drain/heal", strconv.Itoa(len(h14))+" "+h14, "Logplex-Drain-Token", e.app("heal").DrainToken)
	if e.fake.Running("heal")["web"] != 2 {
		t.Fatalf("awake app with stopped dynos was not repaired: %v", e.fake.Running("heal"))
	}
}

func TestRemovedAppIsOptedOutOfAutoEnroll(t *testing.T) {
	e := setup(t)
	e.create("gone", web1)
	e.do("DELETE", "/app/gone", nil)
	var n int64
	e.db.Model(&models.EnrollOptOut{}).Where("heroku_app_name = ?", "gone").Count(&n)
	if n != 1 {
		t.Fatal("removed app not recorded as opted out")
	}
	e.create("gone", web1) // adding it by hand again clears the opt-out
	e.db.Model(&models.EnrollOptOut{}).Where("heroku_app_name = ?", "gone").Count(&n)
	if n != 0 {
		t.Fatal("manual re-add did not clear opt-out")
	}
}
