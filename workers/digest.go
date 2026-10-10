package workers

import (
	"autosleep/alerts"
	"autosleep/metrics"
	"autosleep/models"
	"autosleep/savings"
	"autosleep/sleeper"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gocraft/work"
	"gorm.io/gorm"
)

const (
	DigestJob  = "digest"
	DigestSpec = "0 0 9 * * 1" // Mondays 09:00 UTC
)

// digestEnabled reports whether the weekly savings digest is sent: it needs
// ALERT_WEBHOOK_URL and can be turned off with WEEKLY_DIGEST=false.
func digestEnabled() bool {
	return alerts.Enabled() && !strings.EqualFold(os.Getenv("WEEKLY_DIGEST"), "false")
}

// Digest posts the weekly savings summary.
func (c *Context) Digest(job *work.Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()
	return metrics.Job(DigestJob, alerts.Send(ctx, "weekly_digest", "", DigestText(models.DB, sleeper.Now())))
}

// DigestText summarises the last 7 days: money saved, the biggest savers,
// and apps that need attention.
func DigestText(db *gorm.DB, now time.Time) string {
	var apps []models.Application
	db.Find(&apps)
	from := now.AddDate(0, 0, -7)
	type row struct {
		name  string
		saved float64
	}
	var rows []row
	var saved, hours, monthly float64
	var problems []string
	for i := range apps {
		r := savings.ForApp(db, &apps[i], from, now)
		saved += r.SavedUSD
		hours += r.SleptDynoHours
		monthly += r.ProjectedMonthUSD
		rows = append(rows, row{apps[i].HerokuAppName, r.SavedUSD})
		if apps[i].LastError != "" {
			problems = append(problems, apps[i].HerokuAppName)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].saved > rows[j].saved })

	var b strings.Builder
	noun := "apps"
	if len(apps) == 1 {
		noun = "app"
	}
	fmt.Fprintf(&b, ":crescent_moon: *AutoSleep weekly*: saved *$%.2f* in the last 7 days across %d %s (%.0f dyno-hours asleep, about $%.0f/month).", saved, len(apps), noun, hours, monthly)
	var top []string
	for _, r := range rows {
		if len(top) == 3 || r.saved <= 0 {
			break
		}
		top = append(top, fmt.Sprintf("%s $%.2f", r.name, r.saved))
	}
	if len(top) > 0 {
		b.WriteString("\nTop savers: " + strings.Join(top, ", ") + ".")
	}
	if len(problems) > 0 {
		b.WriteString("\nNeeds attention: " + strings.Join(problems, ", ") + ".")
	}
	if u := sleeper.PublicURL(); u != "" {
		b.WriteString("\n" + u)
	}
	return b.String()
}
