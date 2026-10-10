// Package web serves the AutoSleep dashboard, a single self-contained HTML page
// that talks to the JSON API on the same origin.
package web

import (
	_ "embed" // required for go:embed
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed index.html
var indexHTML []byte

// Dashboard writes the embedded dashboard page. It is expected to be mounted at
// GET / behind basic auth, so the page's same-origin fetches reuse the browser's
// credentials.
func Dashboard(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}
