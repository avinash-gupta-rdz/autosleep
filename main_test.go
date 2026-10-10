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
