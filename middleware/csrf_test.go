package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSameOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SameOrigin())
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	cases := []struct {
		method  string
		headers map[string]string
		want    int
	}{
		{"POST", nil, 200}, // curl
		{"POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://as.example.com"}, 200},
		{"POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"POST", map[string]string{"Origin": "https://evil.example"}, 403},
		{"POST", map[string]string{"Origin": "null"}, 403},
		{"GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, 200},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "https://as.example.com/x", nil)
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Errorf("%s %v = %d, want %d", c.method, c.headers, w.Code, c.want)
		}
	}
}
