package sleeper

import (
	"autosleep/constants"
	"autosleep/herokuclient"
	"autosleep/models"
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	publicURLMu sync.RWMutex
	publicURL   string
)

// SetPublicURL remembers this deployment's URL as seen on requests.
func SetPublicURL(u string) {
	publicURLMu.Lock()
	publicURL = u
	publicURLMu.Unlock()
}

// SelfHostURL is SELF_HOST as a URL (https:// is assumed when it has no
// scheme), or "" when it is not set.
func SelfHostURL() string {
	h := strings.TrimRight(constants.SelfHost, "/")
	if h == "" || strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") {
		return h
	}
	return "https://" + h
}

// PublicURL is SELF_HOST if set, else the URL last seen on a request.
func PublicURL() string {
	if h := SelfHostURL(); h != "" {
		return h
	}
	publicURLMu.RLock()
	defer publicURLMu.RUnlock()
	return publicURL
}

// Enroll verifies access to the Heroku app, snapshots its formation,
// installs the log drain and saves the app. If saving fails the drain is
// removed again so nothing is left half-configured.
func Enroll(parent context.Context, db *gorm.DB, app *models.Application, baseURL string) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	token, err := app.GetToken()
	if err != nil {
		return err
	}
	hc := herokuclient.New(token)
	formations, err := hc.ListFormations(ctx, app.HerokuAppName)
	if err != nil {
		return fmt.Errorf("cannot access Heroku app %q with these credentials: %v", app.HerokuAppName, err)
	}
	app.CurrentConfig = ConfigFrom(formations)
	drainURL := fmt.Sprintf("%s/drain/%s", baseURL, app.HerokuAppName)
	drain, err := hc.CreateDrain(ctx, app.HerokuAppName, drainURL)
	if err != nil {
		// A drain left over from an earlier enrolment makes Heroku reject a
		// duplicate URL; adopt it instead.
		existing, lerr := hc.ListDrains(ctx, app.HerokuAppName)
		found := false
		if lerr == nil {
			for _, d := range existing {
				if d.URL == drainURL {
					drain, found = d, true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("adding log drain to %q: %v", app.HerokuAppName, err)
		}
	}
	app.DrainId, app.DrainToken = drain.ID, drain.Token
	if err := db.Create(app).Error; err != nil {
		hc.DeleteDrain(ctx, app.HerokuAppName, drain.ID)
		return fmt.Errorf("saving app: %v", err)
	}
	return nil
}

// AutoEnrollPatterns reads AUTO_ENROLL, a comma separated list of app name
// globs such as "myapp-pr-*,staging-*".
func AutoEnrollPatterns() []string {
	var out []string
	for _, p := range strings.Split(os.Getenv("AUTO_ENROLL"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// AutoEnroll adds every app visible to a connected Heroku account whose
// name matches AUTO_ENROLL — typically review apps — using the default
// settings (AUTO_ENROLL_IDLE_MINUTES overrides the idle timeout). Apps
// deleted on Heroku are forgotten by the sleep checker.
func AutoEnroll(ctx context.Context, db *gorm.DB) {
	patterns := AutoEnrollPatterns()
	if len(patterns) == 0 {
		return
	}
	base := PublicURL()
	if base == "" {
		log.Printf("auto-enroll: set SELF_HOST (or open the dashboard once) so drain URLs can be built")
		return
	}
	ideal := float64(constants.IdealTime)
	if m, err := strconv.ParseFloat(os.Getenv("AUTO_ENROLL_IDLE_MINUTES"), 64); err == nil && m >= 1 {
		ideal = m * 60
	}

	var accounts []models.Account
	db.Find(&accounts)
	var managed, optedOut []string
	db.Model(&models.Application{}).Pluck("heroku_app_name", &managed)
	db.Model(&models.EnrollOptOut{}).Pluck("heroku_app_name", &optedOut)
	known := map[string]bool{}
	for _, m := range append(managed, optedOut...) {
		known[m] = true
	}
	for i := range accounts {
		act := &accounts[i]
		token, err := act.CurrentAccessToken()
		if err != nil {
			log.Printf("auto-enroll: %s: %v", act.Email, err)
			continue
		}
		apps, err := herokuclient.New(token).ListApps(ctx)
		if err != nil {
			log.Printf("auto-enroll: %s: listing apps: %v", act.Email, err)
			continue
		}
		for _, a := range apps {
			if known[a.Name] || !matchesAny(a.Name, patterns) {
				continue
			}
			app := &models.Application{
				HerokuAppName: a.Name, AccountId: &act.ID, Account: *act, CurrentStatus: true,
				RecentActivityAt: Now(), IdealTime: ideal, CheckInterval: constants.CheckInterval,
			}
			if err := Enroll(ctx, db, app, base); err != nil {
				log.Printf("auto-enroll: %s: %v", a.Name, err)
				continue
			}
			known[a.Name] = true
			log.Printf("auto-enroll: now managing %s", a.Name)
			ScheduleNextCheck(app)
		}
	}
}

// OptOut stops AUTO_ENROLL from managing an app again.
func OptOut(db *gorm.DB, name string) {
	db.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.EnrollOptOut{HerokuAppName: name})
}

// OptIn clears an opt-out, e.g. when the user adds the app by hand.
func OptIn(db *gorm.DB, name string) {
	db.Where("heroku_app_name = ?", name).Delete(&models.EnrollOptOut{})
}
