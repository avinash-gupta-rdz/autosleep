package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Roles. Viewers can see everything but change nothing.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// User is a dashboard/API login.
type User struct {
	Name, Password, Role string
}

// ParseUsers reads USERS, a comma separated list of name:password:role
// (role is admin or viewer, default viewer), e.g.
// "alice:s3cret:admin,qa:hunter2:viewer". Passwords may not contain commas.
func ParseUsers(list string) ([]User, error) {
	var out []User
	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("USERS entry %q must be name:password[:admin|viewer]", parts[0])
		}
		u := User{Name: parts[0], Password: parts[1], Role: RoleViewer}
		if len(parts) == 3 {
			u.Role = strings.ToLower(parts[2])
		}
		if u.Role != RoleAdmin && u.Role != RoleViewer {
			return nil, fmt.Errorf("USERS entry %q: role must be admin or viewer", u.Name)
		}
		out = append(out, u)
	}
	return out, nil
}

const userKey = "autosleep.user"

// BasicAuth checks HTTP basic credentials against users and records the
// user on the context. Every comparison runs, so timing doesn't reveal
// which names exist.
func BasicAuth(users []User) gin.HandlerFunc {
	return func(c *gin.Context) {
		name, pass, ok := c.Request.BasicAuth()
		var found *User
		for i := range users {
			nameOK := subtle.ConstantTimeCompare([]byte(name), []byte(users[i].Name))
			passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(users[i].Password))
			if ok && nameOK&passOK == 1 {
				found = &users[i]
			}
		}
		if found == nil {
			c.Header("WWW-Authenticate", `Basic realm="AutoSleep", charset="UTF-8"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "log in with your AutoSleep user name and password"})
			return
		}
		c.Set(userKey, *found)
	}
}

// CurrentUser returns the logged-in user.
func CurrentUser(c *gin.Context) User {
	u, _ := c.Get(userKey)
	user, _ := u.(User)
	return user
}

// ViewersReadOnly rejects changes from viewers.
func ViewersReadOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return
		}
		if CurrentUser(c).Role != RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "your login is read-only; ask an admin to make this change"})
		}
	}
}
