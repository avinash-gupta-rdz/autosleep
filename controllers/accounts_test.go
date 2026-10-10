package controllers

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"errors"
	"net/http"
	"strconv"
	"testing"
)

func TestConnectAccountWithAPIKey(t *testing.T) {
	e := setup(t)
	e.fake.Apps = []herokuclient.App{{Name: "staging"}, {Name: "demo"}}
	e.fake.Formations["staging"] = []herokuclient.Formation{{Type: "web", Size: "basic", Quantity: 1}}

	e.fake.FailNext["AccountInfo"] = errors.New("401 unauthorized")
	if code, _ := e.do("POST", "/accounts", map[string]string{"api_key": "bad"}); code != http.StatusBadRequest {
		t.Fatalf("bad key: %d", code)
	}

	code, out := e.do("POST", "/accounts", map[string]string{"api_key": " good-key \n"})
	if code != http.StatusOK {
		t.Fatalf("connect: %d %v", code, out)
	}
	var act models.Account
	e.db.First(&act)
	if act.AuthType != models.AuthAPIKey || act.Email != "dev@example.com" {
		t.Fatalf("stored account = %+v", act)
	}
	// API keys never go through OAuth refresh, even with no expiry set.
	if tok, err := act.CurrentAccessToken(); err != nil || tok != "good-key" {
		t.Fatalf("token = %q, %v", tok, err)
	}

	id := strconv.Itoa(int(act.ID))
	code, out = e.do("GET", "/accounts/"+id+"/apps", nil)
	if code != http.StatusOK || len(out["list"].([]interface{})) != 2 {
		t.Fatalf("list apps: %d %v", code, out)
	}
	if code, out := e.do("POST", "/app", map[string]interface{}{"heroku_app_name": "staging", "account_id": act.ID}); code != http.StatusOK {
		t.Fatalf("enrol via account: %d %v", code, out)
	}

	// Disconnecting is refused while an app depends on the account.
	if code, _ := e.do("DELETE", "/accounts/"+id, nil); code != http.StatusConflict {
		t.Fatalf("delete in use: %d", code)
	}
	if code, _ := e.do("DELETE", "/app/staging", nil); code != http.StatusOK {
		t.Fatalf("remove app: %d", code)
	}
	if code, _ := e.do("DELETE", "/accounts/"+id, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	// It can be connected again afterwards (no unique-index leftovers).
	if code, _ := e.do("POST", "/accounts", map[string]string{"api_key": "good-key"}); code != http.StatusOK {
		t.Fatalf("reconnect: %d", code)
	}
}
