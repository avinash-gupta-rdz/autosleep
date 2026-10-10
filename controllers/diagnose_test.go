package controllers

import (
	"net/http"
	"strconv"
	"testing"

	heroku "github.com/heroku/heroku-go/v5"
)

func checks(t *testing.T, e *env, app string) map[string]map[string]interface{} {
	t.Helper()
	code, out := e.do("GET", "/app/"+app+"/diagnose", nil)
	if code != 200 {
		t.Fatalf("diagnose: %d %v", code, out)
	}
	m := map[string]map[string]interface{}{}
	for _, c := range out["list"].([]interface{}) {
		c := c.(map[string]interface{})
		m[c["name"].(string)] = c
	}
	return m
}

func TestDiagnoseAndRepairDrain(t *testing.T) {
	e := setup(t)
	e.create("shop", web1)

	got := checks(t, e, "shop")
	if got["Heroku access"]["status"] != "ok" || got["Log drain"]["status"] != "ok" {
		t.Fatalf("fresh app: %v", got)
	}
	if got["Logs arriving"]["status"] != "warn" {
		t.Fatalf("no logs yet should warn: %v", got["Logs arriving"])
	}

	// A log batch (even a non-traffic one) is recorded as a heartbeat.
	line := `<190>1 2026-10-07T06:45:26+00:00 host app web.1 - hello`
	e.do("POST", "/drain/shop", strconv.Itoa(len(line))+" "+line, "Logplex-Drain-Token", e.app("shop").DrainToken)
	if e.app("shop").LastDrainAt == nil {
		t.Fatal("last_drain_at not recorded")
	}
	if got := checks(t, e, "shop"); got["Logs arriving"]["status"] != "ok" {
		t.Fatalf("after logs: %v", got["Logs arriving"])
	}

	// Someone deletes the drain on Heroku: diagnose offers a repair.
	e.fake.Drains["shop"] = nil
	got = checks(t, e, "shop")
	if got["Log drain"]["status"] != "fail" || got["Log drain"]["action"] != "repair-drain" {
		t.Fatalf("missing drain: %v", got["Log drain"])
	}
	if code, out := e.do("POST", "/app/shop/repair-drain", nil); code != 200 {
		t.Fatalf("repair: %d %v", code, out)
	}
	if len(e.fake.Drains["shop"]) != 1 || e.app("shop").DrainId != e.fake.Drains["shop"][0].ID {
		t.Fatalf("drain not reinstalled: %v / %s", e.fake.Drains["shop"], e.app("shop").DrainId)
	}
	if got := checks(t, e, "shop"); got["Log drain"]["status"] != "ok" {
		t.Fatalf("after repair: %v", got["Log drain"])
	}
}

func TestDiagnoseDeletedHerokuApp(t *testing.T) {
	e := setup(t)
	e.create("gone", web1)
	e.fake.FailNext["ListFormations"] = herokuNotFound()
	if got := checks(t, e, "gone"); got["Heroku access"]["status"] != "fail" {
		t.Fatalf("deleted app: %v", got)
	}
}

func TestSetupStatus(t *testing.T) {
	e := setup(t)
	_, out := e.do("GET", "/setup", nil)
	s := out["data"].(map[string]interface{})
	if s["apps"].(float64) != 0 || s["logs_received"].(bool) || s["slept_once"].(bool) {
		t.Fatalf("empty setup: %v", s)
	}
	e.create("shop", web1)
	e.do("POST", "/app/shop/sleep", nil)
	_, out = e.do("GET", "/setup", nil)
	s = out["data"].(map[string]interface{})
	if s["apps"].(float64) != 1 || !s["slept_once"].(bool) {
		t.Fatalf("after first sleep: %v", s)
	}
}

func herokuNotFound() error { return heroku.Error{StatusCode: http.StatusNotFound} }
