// Package demo runs AutoSleep without Postgres, Redis or a Heroku account:
// an in-memory database and Redis, and a simulated Heroku with a handful of
// apps and some traffic. It exists so people can try the dashboard in one
// command (DEMO_MODE=true) before deploying for real.
package demo

import (
	"autosleep/constants"
	"autosleep/herokuclient"
	"autosleep/herokuclient/fake"
	"autosleep/models"
	"autosleep/sleeper"
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/gomodule/redigo/redis"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Enabled reports whether DEMO_MODE is on.
func Enabled() bool {
	v := strings.ToLower(os.Getenv("DEMO_MODE"))
	return v == "true" || v == "1" || v == "yes"
}

// Defaults fills in the config a real deployment must provide, so
// `DEMO_MODE=true go run .` works with nothing else set.
func Defaults(port string) {
	for k, v := range map[string]string{"API_USER": "demo", "API_PASS": "demo", "PASSPHRASE": "demo-mode-passphrase"} {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	constants.Passphrase = os.Getenv("PASSPHRASE")
	if constants.SelfHost == "" && os.Getenv("DYNO") == "" {
		constants.SelfHost = "http://localhost:" + port
	}
}

// Apps in the simulated Heroku account: name, process types and sizes.
var apps = []struct {
	name       string
	formation  []herokuclient.Formation
	enrolled   bool
	idleMinute float64
}{
	{"acme-staging", []herokuclient.Formation{{Type: "web", Size: "Standard-1X", Quantity: 2}, {Type: "worker", Size: "Standard-1X", Quantity: 1}}, true, 3},
	{"acme-demo", []herokuclient.Formation{{Type: "web", Size: "Basic", Quantity: 1}}, true, 2},
	{"acme-pr-412", []herokuclient.Formation{{Type: "web", Size: "Standard-2X", Quantity: 1}}, true, 2},
	{"acme-pr-415", []herokuclient.Formation{{Type: "web", Size: "Standard-1X", Quantity: 1}}, false, 0},
	{"acme-docs", []herokuclient.Formation{{Type: "web", Size: "Basic", Quantity: 1}}, false, 0},
	{"acme-production", []herokuclient.Formation{{Type: "web", Size: "Performance-M", Quantity: 3}}, false, 0},
}

// Setup installs the in-memory database, Redis and fake Heroku and seeds a
// demo account with history. Call it instead of ConnectDataBase/RedisConn.
func Setup() {
	db, err := gorm.Open(sqlite.Open("file:autosleep-demo?mode=memory&cache=shared&_pragma=busy_timeout(5000)"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("demo: database: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1) // SQLite in memory: one writer
	if err := models.Migrate(db); err != nil {
		log.Fatalf("demo: migrate: %v", err)
	}
	models.DB = db

	mr, err := miniredis.Run()
	if err != nil {
		log.Fatalf("demo: redis: %v", err)
	}
	models.REDIS = &redis.Pool{MaxActive: 20, Wait: true, Dial: func() (redis.Conn, error) { return redis.Dial("tcp", mr.Addr()) }}

	f := &fake.Heroku{Email: "you@acme.example", Name: "Acme Dev"}
	f.Activate()
	for _, a := range apps {
		f.Apps = append(f.Apps, herokuclient.App{Name: a.name, WebURL: "https://" + a.name + ".herokuapp.com/"})
		f.Formations[a.name] = append([]herokuclient.Formation(nil), a.formation...)
	}
	log.Printf("DEMO MODE: in-memory data and a simulated Heroku account; nothing real is scaled. Log in as %s / %s.",
		os.Getenv("API_USER"), os.Getenv("API_PASS"))
}

// Seed enrols the demo apps and backfills 30 days of sleep/wake history so
// the savings report has something to show. baseURL is this server's URL.
func Seed(baseURL string) {
	act := models.Account{Email: "you@acme.example", Name: "Acme Dev", AuthType: models.AuthAPIKey, AccessToken: models.Encrypt("demo-key")}
	if err := models.DB.Create(&act).Error; err != nil {
		log.Printf("demo: seeding account: %v", err)
		return
	}
	sleeper.SetPublicURL(baseURL)
	now := time.Now().UTC()
	for _, a := range apps {
		if !a.enrolled {
			continue
		}
		app := &models.Application{
			HerokuAppName: a.name, AccountId: &act.ID, Account: act, CurrentStatus: true,
			RecentActivityAt: now, IdealTime: a.idleMinute * 60, CheckInterval: 60, WakePage: true,
		}
		if err := sleeper.Enroll(context.Background(), models.DB, app, baseURL); err != nil {
			log.Printf("demo: enrolling %s: %v", a.name, err)
			continue
		}
		models.DB.Model(app).Updates(map[string]interface{}{"created_at": now.AddDate(0, 0, -30), "last_drain_at": now})
		backfill(app, a.formation, now)
		sleeper.ScheduleNextCheck(app)
	}
}

// backfill writes a month of office-hours usage: awake roughly 9:00-18:00
// on weekdays, asleep otherwise.
func backfill(app *models.Application, f []herokuclient.Formation, now time.Time) {
	for d := 30; d >= 1; d-- {
		day := now.AddDate(0, 0, -d).Truncate(24 * time.Hour)
		if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		wake := day.Add(9*time.Hour + time.Duration(rand.IntN(60))*time.Minute)
		sleep := day.Add(18*time.Hour + time.Duration(rand.IntN(120))*time.Minute)
		for _, p := range f {
			models.DB.Create(&models.AppHistory{ApplicationId: app.ID, Action: models.ActionWake, DynoName: p.Type, DynoSize: p.Size, Quantity: p.Quantity, Reason: sleeper.ReasonTraffic, PerformedAt: wake})
			models.DB.Create(&models.AppHistory{ApplicationId: app.ID, Action: models.ActionSleep, DynoName: p.Type, DynoSize: p.Size, Quantity: p.Quantity, Reason: sleeper.ReasonIdle, PerformedAt: sleep})
		}
	}
}

// Traffic posts a fake router log line for a random enrolled app every
// minute or so, the way Heroku's log drain would, so visitors see apps wake
// up and fall asleep again on their own.
func Traffic(port string) {
	for {
		time.Sleep(time.Duration(40+rand.IntN(50)) * time.Second)
		var list []models.Application
		models.DB.Find(&list)
		if len(list) == 0 {
			continue
		}
		app := list[rand.IntN(len(list))]
		line := fmt.Sprintf(`<158>1 %s host heroku router - at=info method=GET path="/" host=%s.herokuapp.com status=200`, time.Now().UTC().Format(time.RFC3339), app.HerokuAppName)
		body := strconv.Itoa(len(line)) + " " + line
		req, _ := http.NewRequest("POST", "http://127.0.0.1:"+port+"/drain/"+app.HerokuAppName, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/logplex-1")
		req.Header.Set("Logplex-Drain-Token", app.DrainToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}
}
