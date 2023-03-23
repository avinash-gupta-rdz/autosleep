package middleware

import (
	"log"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// DisableLogging set up logging. default is false (logging)
var DisableLogging bool

// IPWhiteList is a middleware that check given CIDR rules and return 403 Forbidden
// when user is not coming from allowed source. CIDRs accepts a list of CIDRs,
// separated by comma. (e.g. 127.0.0.1/32, ::1/128 )
func IPWhiteList(CIDRs string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// parse it into IP type
		remoteIP := net.ParseIP(c.ClientIP())

		// split CIDRs by comma, and we gonna check them one by one
		cidrSlices := strings.Split(CIDRs, ",")

		// under of CIDR we were in
		var matchCount uint

		// go over each CIDR and do the tests
		for _, cidr := range cidrSlices {
			// remove unwanted spaces
			cidr = strings.TrimSpace(cidr)

			// try to parse the CIDR
			_, cidrIPNet, parseCIDRErr := net.ParseCIDR(cidr)

			if parseCIDRErr != nil {
				c.AbortWithError(500, parseCIDRErr)
				return
			}

			// This is the core of this middleware,
			// it ask current CIDR network range to test if current IP is in
			if cidrIPNet.Contains(remoteIP) {
				matchCount = matchCount + 1
			}
		}

		// if no CIDR ranges contains our IP
		if matchCount == 0 {
			if DisableLogging == false {
				log.Printf("[LIMIT] Request from [" + c.ClientIP() + "] is not allow to access `" + c.Request.RequestURI + "`, only allow from: [" + CIDRs + "]")
			}

			c.AbortWithStatus(403)
			return
		}
	}
}
