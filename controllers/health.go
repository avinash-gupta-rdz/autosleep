package controllers

import (
	"autosleep/models"
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Health reports whether the database and Redis are reachable.
func Health(c *gin.Context) {
	status := gin.H{"database": "ok", "redis": "ok"}
	code := http.StatusOK
	if sqlDB, err := models.DB.DB(); err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
		status["database"], code = "unreachable", http.StatusServiceUnavailable
	}
	if models.REDIS != nil {
		// The pool waits for a free connection; don't let /health hang.
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		conn, err := models.REDIS.GetContext(ctx)
		if err == nil {
			_, err = conn.Do("PING")
			conn.Close()
		}
		if err != nil {
			status["redis"], code = "unreachable", http.StatusServiceUnavailable
		}
	}
	c.JSON(code, status)
}

// Liveness answers 200 while the process runs, without touching the
// database or Redis, so a dependency outage doesn't get the dyno restarted.
// /readyz (an alias of /health) reports dependency status.
func Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
