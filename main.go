package main

import (
	"autosleep/constants"
	"autosleep/controllers"
	"autosleep/demo"
	"autosleep/metrics"
	"autosleep/middleware"
	"autosleep/models"
	"autosleep/savings"
	"autosleep/schedule"
	"autosleep/sleeper"
	"autosleep/web"
	"autosleep/workers"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	demoMode := demo.Enabled()
	if demoMode {
		demo.Defaults(port())
	}
	checkConfig()
	if demoMode {
		demo.Setup()
	} else {
		models.ConnectDataBase()
		models.RedisConn()
	}
	sleeper.Checks = workers.Queue{Pool: models.REDIS}
	connectEnvAPIKey()

	concurrency := 10
	if n, err := strconv.Atoi(os.Getenv("WORKER_CONCURRENCY")); err == nil && n > 0 {
		concurrency = n
	}
	pool := workers.NewPool(models.REDIS, uint(concurrency))
	pool.Start()
	log.Printf("Worker started (night window %s)", sleeper.Night)

	srv := &http.Server{Addr: ":" + port(), Handler: router(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("Listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	if demoMode {
		go func() {
			base := sleeper.PublicURL()
			if base == "" {
				base = "http://localhost:" + port()
			}
			demo.Seed(base)
			demo.Traffic(port())
		}()
	}

	// Heroku sends SIGTERM on restarts and deploys.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan
	log.Println("Shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	controllers.Pending.Wait() // let in-flight wake-ups finish
	pool.Stop()
}

// checkConfig fails fast with an actionable message instead of crashing
// later on the first request.
func checkConfig() {
	var missing []string
	for _, k := range []string{"API_USER", "API_PASS", "PASSPHRASE"} {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		log.Fatalf("missing required config vars: %s (see README)", strings.Join(missing, ", "))
	}
	night, err := schedule.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	sleeper.Night = night
	savings.LoadPriceOverrides()
}

// connectEnvAPIKey connects the account for HEROKU_API_KEY at startup, so a
// fresh Deploy-button install can list and enrol apps right away.
func connectEnvAPIKey() {
	key := os.Getenv("HEROKU_API_KEY")
	if key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	act, err := controllers.ConnectAPIKey(ctx, key)
	if err != nil {
		log.Printf("HEROKU_API_KEY: %v", err)
		return
	}
	log.Printf("Connected Heroku account %s from HEROKU_API_KEY", act.Email)
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}

// quietLogger logs requests except successful log-drain posts, which
// arrive constantly and would drown everything else.
func quietLogger() gin.HandlerFunc {
	logger := gin.Logger()
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/drain/") || isProbe(c.Request.URL.Path) {
			c.Next()
			if c.Writer.Status() < 400 {
				return
			}
			log.Printf("%s %s -> %d", c.Request.Method, c.Request.URL.Path, c.Writer.Status())
			return
		}
		logger(c)
	}
}

func isProbe(path string) bool {
	switch path {
	case "/health", "/healthz", "/readyz", "/metrics":
		return true
	}
	return false
}

func router() *gin.Engine {
	r := gin.New()
	r.Use(quietLogger(), gin.Recovery())

	r.Use(metrics.Recorder())

	r.POST("/drain/:app_id", controllers.ProcessDrain)
	r.GET("/health", controllers.Health)
	r.GET("/readyz", controllers.Health)
	r.GET("/healthz", controllers.Liveness)
	// Browser-facing public pages get a per-IP limit; the drain does not
	// (Logplex posts for all apps come from a few shared IPs).
	public := middleware.RateLimit(5, 20)
	r.GET("/auth/heroku/callback", public, controllers.HandleAuthCallback)
	r.GET("/waking/:app_id", public, controllers.WakingPage)

	// IP allowlist first, so outsiders can't even probe passwords.
	authorized := r.Group("/", middleware.IPWhiteList(constants.AllowedIps), gin.BasicAuth(gin.Accounts{
		os.Getenv("API_USER"): os.Getenv("API_PASS"),
	}), middleware.SameOrigin())
	authorized.GET("/", web.Dashboard)
	authorized.GET("/metrics", metrics.Handler)
	authorized.GET("/register", controllers.HandleAuth)
	authorized.GET("/app", controllers.FindApplications)
	authorized.POST("/app", controllers.CreateApp)
	authorized.GET("/app/:app_id", controllers.FindApp)
	authorized.PATCH("/app/:app_id", controllers.UpdateApp)
	authorized.DELETE("/app/:app_id", controllers.DeleteApp)
	authorized.POST("/app/:app_id/wake", controllers.WakeApp)
	authorized.POST("/app/:app_id/sleep", controllers.SleepApp)
	authorized.POST("/app/:app_id/pause", controllers.PauseApp)
	authorized.POST("/app/:app_id/resume", controllers.ResumeApp)
	authorized.GET("/app/:app_id/history", controllers.AppHistory)
	authorized.GET("/app/:app_id/diagnose", controllers.DiagnoseApp)
	authorized.POST("/app/:app_id/repair-drain", controllers.RepairDrain)
	authorized.GET("/setup", controllers.Setup)
	authorized.GET("/app/:app_id/savings", controllers.AppSavings)
	authorized.GET("/savings", controllers.AllSavings)
	authorized.GET("/accounts", controllers.ListAccounts)
	authorized.POST("/accounts", controllers.CreateAccount)
	authorized.DELETE("/accounts/:account_id", controllers.DeleteAccount)
	authorized.GET("/accounts/:account_id/apps", controllers.AccountApps)
	return r
}
