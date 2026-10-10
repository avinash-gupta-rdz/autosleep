package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUsersAndRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users, err := ParseUsers("alice:pw1:admin, qa:pw2 ,")
	if err != nil || len(users) != 2 || users[1].Role != RoleViewer {
		t.Fatalf("ParseUsers = %+v, %v", users, err)
	}
	for _, bad := range []string{"nopass", "x:y:root", ":pw"} {
		if _, err := ParseUsers(bad); err == nil {
			t.Errorf("ParseUsers(%q) accepted", bad)
		}
	}

	r := gin.New()
	r.Use(BasicAuth(users), ViewersReadOnly())
	r.GET("/x", func(c *gin.Context) { c.String(200, CurrentUser(c).Role) })
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	do := func(method, user, pass string) int {
		req := httptest.NewRequest(method, "/x", nil)
		if user != "" {
			req.SetBasicAuth(user, pass)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	cases := []struct {
		method, user, pass string
		want               int
	}{
		{"GET", "", "", http.StatusUnauthorized},
		{"GET", "alice", "wrong", http.StatusUnauthorized},
		{"GET", "qa", "pw1", http.StatusUnauthorized},
		{"GET", "qa", "pw2", http.StatusOK},
		{"POST", "qa", "pw2", http.StatusForbidden},
		{"POST", "alice", "pw1", http.StatusOK},
	}
	for _, c := range cases {
		if got := do(c.method, c.user, c.pass); got != c.want {
			t.Errorf("%s as %s/%s = %d, want %d", c.method, c.user, c.pass, got, c.want)
		}
	}
}
