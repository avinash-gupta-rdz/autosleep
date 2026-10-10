package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRateLimitPerIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(0.001, 2))
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	get := func(remote string) int {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remote
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 2; i++ {
		if c := get("1.2.3.4:1"); c != http.StatusOK {
			t.Fatalf("request %d within burst: %d", i, c)
		}
	}
	if c := get("1.2.3.4:1"); c != http.StatusTooManyRequests {
		t.Fatalf("over burst: %d", c)
	}
	if c := get("5.6.7.8:1"); c != http.StatusOK {
		t.Fatalf("other IP limited: %d", c)
	}
}
