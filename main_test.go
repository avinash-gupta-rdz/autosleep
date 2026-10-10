package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProbesAndMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_USER", "admin")
	t.Setenv("API_PASS", "pw")
	r := router()

	do := func(path string, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if auth {
			req.SetBasicAuth("admin", "pw")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := do("/healthz", false); w.Code != http.StatusOK {
		t.Errorf("/healthz = %d", w.Code)
	}
	if w := do("/metrics", false); w.Code != http.StatusUnauthorized {
		t.Errorf("/metrics without auth = %d, want 401", w.Code)
	}
	w := do("/metrics", true)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "autosleep_http_requests_total") {
		t.Errorf("/metrics = %d, body missing autosleep metrics", w.Code)
	}
}

func TestViewerLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_USER", "admin")
	t.Setenv("API_PASS", "pw")
	t.Setenv("USERS", "qa:look:viewer")
	r := router()
	req := httptest.NewRequest("GET", "/me", nil)
	req.SetBasicAuth("qa", "look")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"role":"viewer"`) {
		t.Fatalf("/me as viewer = %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest("POST", "/app/x/sleep", nil)
	req.SetBasicAuth("qa", "look")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer POST = %d, want 403", w.Code)
	}
}
