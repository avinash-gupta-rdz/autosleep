package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimit is a per-client-IP token bucket: rps requests per second on
// average, bursts of up to burst. It is meant for unauthenticated pages a
// browser opens by hand (OAuth callback, waking page). Don't put it on the
// log drain: Logplex posts for every app arrive from a few shared IPs.
func RateLimit(rps float64, burst int) gin.HandlerFunc {
	l := &ipLimiter{visitors: map[string]*visitor{}, rps: rate.Limit(rps), burst: burst}
	return func(c *gin.Context) {
		if !l.allow(ClientIP(c), time.Now()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests, slow down"})
			return
		}
		c.Next()
	}
}

// visitorTTL is how long an idle IP's bucket is kept.
const visitorTTL = 10 * time.Minute

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipLimiter struct {
	mu        sync.Mutex
	visitors  map[string]*visitor
	rps       rate.Limit
	burst     int
	lastSweep time.Time
}

func (l *ipLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Sweep inline instead of in a goroutine, so the map stays bounded
	// without a background loop per middleware instance.
	if now.Sub(l.lastSweep) > visitorTTL {
		for k, v := range l.visitors {
			if now.Sub(v.lastSeen) > visitorTTL {
				delete(l.visitors, k)
			}
		}
		l.lastSweep = now
	}
	v, ok := l.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.visitors[ip] = v
	}
	v.lastSeen = now
	return v.limiter.AllowN(now, 1)
}
