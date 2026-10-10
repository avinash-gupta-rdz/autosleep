package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func serve(t *testing.T, cidrs, remote, xff string) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(IPWhiteList(cidrs))
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestAllowlist(t *testing.T) {
	DisableLogging = true
	if c := serve(t, "10.0.0.0/8", "10.1.2.3:1234", ""); c != http.StatusOK {
		t.Errorf("inside range: %d", c)
	}
	if c := serve(t, "10.0.0.0/8", "8.8.8.8:1234", ""); c != http.StatusForbidden {
		t.Errorf("outside range: %d", c)
	}
	if c := serve(t, "", "8.8.8.8:1234", ""); c != http.StatusOK {
		t.Errorf("empty list should allow: %d", c)
	}
	if c := serve(t, "8.8.8.8", "8.8.8.8:1234", ""); c != http.StatusOK {
		t.Errorf("bare IP should work: %d", c)
	}
}

func TestSpoofedForwardedForOnHeroku(t *testing.T) {
	DisableLogging = true
	t.Setenv("DYNO", "web.1")
	// Attacker sends an allowed IP first; the router appends the real one.
	if c := serve(t, "10.0.0.0/8", "172.16.0.1:1234", "10.0.0.1, 8.8.8.8"); c != http.StatusForbidden {
		t.Errorf("spoofed XFF was trusted: %d", c)
	}
	if c := serve(t, "10.0.0.0/8", "172.16.0.1:1234", "8.8.8.8, 10.0.0.1"); c != http.StatusOK {
		t.Errorf("real client IP rejected: %d", c)
	}
}

func TestParseCIDRsRejectsGarbage(t *testing.T) {
	if _, err := ParseCIDRs("10.0.0.0/8, nonsense"); err == nil {
		t.Fatal("expected error")
	}
}
