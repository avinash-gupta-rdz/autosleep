// Package metrics defines the Prometheus metrics AutoSleep exports at
// /metrics. Labels are kept to small, fixed sets so cardinality stays
// bounded however many apps are managed.
package metrics

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ScaleEvents counts successful sleeps and wakes.
	ScaleEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "autosleep", Subsystem: "scale", Name: "events_total",
		Help: "Successful scale events, by action (sleep|wake) and reason.",
	}, []string{"action", "reason"})

	// AppErrors counts failed sleep/wake/enrol attempts recorded on an app.
	AppErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "autosleep", Subsystem: "app", Name: "errors_total",
		Help: "Errors recorded against managed apps (failed Heroku calls, missing credentials).",
	})

	// DrainRejected counts log drain posts that were refused.
	DrainRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "autosleep", Subsystem: "drain", Name: "rejected_total",
		Help: "Rejected log drain posts, by reason (unknown_app|bad_token).",
	}, []string{"reason"})

	// WorkerJobs counts background jobs by outcome.
	WorkerJobs = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "autosleep", Subsystem: "worker", Name: "jobs_total",
		Help: "Background jobs, by name and outcome (ok|error).",
	}, []string{"name", "outcome"})

	// HTTPRequests counts requests by route pattern and status class.
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "autosleep", Subsystem: "http", Name: "requests_total",
		Help: "HTTP requests, by route and status class (2xx, 4xx, ...).",
	}, []string{"route", "status"})
)

func init() {
	prometheus.MustRegister(ScaleEvents, AppErrors, DrainRejected, WorkerJobs, HTTPRequests)
}

// Job records the outcome of a background job and returns err unchanged.
func Job(name string, err error) error {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	WorkerJobs.WithLabelValues(name, outcome).Inc()
	return err
}

// Recorder is gin middleware that counts requests. It uses the matched
// route pattern (/app/:app_id), never the raw path.
func Recorder() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		HTTPRequests.WithLabelValues(route, strconv.Itoa(c.Writer.Status()/100)+"xx").Inc()
	}
}

var handler = promhttp.Handler()

// Handler serves the Prometheus text format.
func Handler(c *gin.Context) { handler.ServeHTTP(c.Writer, c.Request) }
