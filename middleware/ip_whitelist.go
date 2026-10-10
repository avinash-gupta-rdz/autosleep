package middleware

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// DisableLogging silences the log line written for each rejected request.
var DisableLogging bool

// ParseCIDRs parses a comma separated CIDR list such as
// "127.0.0.1/32, ::1/128". Bare IPs are accepted as single-host ranges.
func ParseCIDRs(list string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, cidr := range strings.Split(list, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if !strings.Contains(cidr, "/") {
			if ip := net.ParseIP(cidr); ip != nil && ip.To4() != nil {
				cidr += "/32"
			} else {
				cidr += "/128"
			}
		}
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q in WHITELISTED_IPS: %w", cidr, err)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// ClientIP returns the caller's address. On Heroku the router appends the
// real client address as the *last* X-Forwarded-For entry; earlier entries
// are whatever the client sent and must not be trusted.
func ClientIP(c *gin.Context) string {
	if os.Getenv("DYNO") != "" {
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return c.RemoteIP()
}

// IPWhiteList is a middleware that returns 403 Forbidden when the caller is
// not inside one of the allowed networks. CIDRs are parsed once at startup;
// an empty list allows everyone (basic auth still applies).
func IPWhiteList(CIDRs string) gin.HandlerFunc {
	nets, err := ParseCIDRs(CIDRs)
	if err != nil {
		log.Fatal(err)
	}
	if len(nets) == 0 {
		return func(c *gin.Context) {}
	}
	return func(c *gin.Context) {
		ip := ClientIP(c)
		remoteIP := net.ParseIP(ip)
		for _, n := range nets {
			if remoteIP != nil && n.Contains(remoteIP) {
				return
			}
		}
		if !DisableLogging {
			log.Printf("[LIMIT] Request from [%s] is not allowed to access `%s`", ip, c.Request.URL.Path)
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "your IP address is not allowed (see WHITELISTED_IPS)"})
	}
}
