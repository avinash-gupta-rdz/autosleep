package controllers

import (
	"autosleep/alerts"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TestAlert posts a test message to ALERT_WEBHOOK_URL so users can check
// the webhook works.
func TestAlert(c *gin.Context) {
	if err := alerts.Send(c.Request.Context(), "test", "", "AutoSleep test alert: notifications are working."); err != nil {
		jsonError(c, http.StatusBadGateway, "Could not send: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "Test alert sent"})
}
