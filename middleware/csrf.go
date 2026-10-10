package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// SameOrigin rejects state-changing requests made by a browser from another
// site. Browsers attach cached basic-auth credentials to cross-site form
// posts, so without this any page could make an admin's browser sleep or
// remove apps. Non-browser clients (curl, scripts) send neither header and
// are unaffected.
func SameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return
		}
		if crossSite(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cross-site request blocked"})
		}
	}
}

func crossSite(c *gin.Context) bool {
	switch c.GetHeader("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return true
	}
	if origin := c.GetHeader("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err != nil || !strings.EqualFold(u.Host, c.Request.Host)
	}
	return false
}
