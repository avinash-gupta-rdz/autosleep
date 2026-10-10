// Package savings estimates how much money sleeping dynos saved, from the
// sleep/wake history AutoSleep records.
package savings

import (
	"autosleep/models"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// HourlyPrice is Heroku's list price in USD per dyno-hour, keyed by the
// lower-cased dyno size (https://www.heroku.com/pricing). Eco dynos draw
// from a flat-rate hour pool, so sleeping them saves hours but not dollars.
// Override or extend with DYNO_HOURLY_PRICES="standard-1x=0.035,private-s=0.175".
var HourlyPrice = map[string]float64{
	"eco":               0,
	"basic":             0.010,
	"standard-1x":       0.035,
	"standard-2x":       0.070,
	"performance-m":     0.350,
	"performance-l":     0.700,
	"performance-l-ram": 0.700,
	"performance-xl":    1.050,
	"performance-2xl":   2.100,
	"private-s":         0.175,
	"private-m":         0.350,
	"private-l":         0.700,
	"private-l-ram":     0.700,
	"private-xl":        1.050,
	"private-2xl":       2.100,
	"shield-s":          0.210,
	"shield-m":          0.420,
	"shield-l":          0.840,
	"shield-l-ram":      0.840,
	"shield-xl":         1.260,
	"shield-2xl":        2.520,
	// Legacy names still returned by the API for old apps.
	"free":        0,
	"hobby":       0.010,
	"1x":          0.035,
	"2x":          0.070,
	"performance": 0.350,
}

// LoadPriceOverrides applies DYNO_HOURLY_PRICES from the environment.
func LoadPriceOverrides() {
	for _, pair := range strings.Split(os.Getenv("DYNO_HOURLY_PRICES"), ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			continue
		}
		if p, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64); err == nil && p >= 0 {
			HourlyPrice[strings.ToLower(strings.TrimSpace(kv[0]))] = p
		}
	}
}

// Price returns the hourly price for a dyno size and whether it is known.
func Price(size string) (float64, bool) {
	p, ok := HourlyPrice[strings.ToLower(size)]
	return p, ok
}

// Interval is one process type sleeping from Start to End.
type Interval struct {
	Type, Size string
	Quantity   int
	Start, End time.Time
}

// Intervals pairs sleep and wake events into sleep intervals. A sleep with
// no wake yet is open and runs until now.
func Intervals(events []models.AppHistory, now time.Time) []Interval {
	sorted := append([]models.AppHistory(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].PerformedAt.Equal(sorted[j].PerformedAt) {
			return sorted[i].PerformedAt.Before(sorted[j].PerformedAt)
		}
		return sorted[i].ID < sorted[j].ID
	})
	var out []Interval
	open := map[string]*Interval{}
	closeAll := func(at time.Time) {
		for k, iv := range open {
			iv.End = at
			out = append(out, *iv)
			delete(open, k)
		}
	}
	for _, e := range sorted {
		if e.Action == models.ActionWake {
			closeAll(e.PerformedAt)
			continue
		}
		if prev, ok := open[e.DynoName]; ok { // two sleeps in a row
			prev.End = e.PerformedAt
			out = append(out, *prev)
		}
		open[e.DynoName] = &Interval{Type: e.DynoName, Size: e.DynoSize, Quantity: e.Quantity, Start: e.PerformedAt}
	}
	closeAll(now)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// Line is the savings for one process type and size.
type Line struct {
	Type      string  `json:"type"`
	Size      string  `json:"size"`
	DynoHours float64 `json:"slept_dyno_hours"`
	HourlyUSD float64 `json:"hourly_price_usd"`
	SavedUSD  float64 `json:"saved_usd"`
	Unpriced  bool    `json:"unpriced,omitempty"`
}

// Report summarises savings over [From, To].
type Report struct {
	From              time.Time `json:"from"`
	To                time.Time `json:"to"`
	SleptDynoHours    float64   `json:"slept_dyno_hours"`
	SavedUSD          float64   `json:"saved_usd"`
	SleepingPercent   float64   `json:"sleeping_percent"`
	ProjectedMonthUSD float64   `json:"projected_monthly_usd"`
	Lines             []Line    `json:"breakdown"`
	UnpricedSizes     []string  `json:"unpriced_sizes,omitempty"`
}

// Compute builds a report for events in the window [from, to].
func Compute(events []models.AppHistory, from, to time.Time) Report {
	r := Report{From: from, To: to, Lines: []Line{}}
	if !to.After(from) {
		return r
	}
	lines := map[string]*Line{}
	var asleep []Interval // app-level coverage, for the percentage
	for _, iv := range Intervals(events, to) {
		s, e := iv.Start, iv.End
		if s.Before(from) {
			s = from
		}
		if e.After(to) {
			e = to
		}
		if !e.After(s) {
			continue
		}
		asleep = append(asleep, Interval{Start: s, End: e})
		hours := e.Sub(s).Hours() * float64(iv.Quantity)
		key := iv.Type + "|" + strings.ToLower(iv.Size)
		l, ok := lines[key]
		if !ok {
			price, known := Price(iv.Size)
			l = &Line{Type: iv.Type, Size: iv.Size, HourlyUSD: price, Unpriced: !known}
			lines[key] = l
		}
		l.DynoHours += hours
	}
	unpriced := map[string]bool{}
	for _, l := range lines {
		l.SavedUSD = round(l.DynoHours*l.HourlyUSD, 2)
		l.DynoHours = round(l.DynoHours, 2)
		r.SleptDynoHours += l.DynoHours
		r.SavedUSD += l.SavedUSD
		if l.Unpriced {
			unpriced[l.Size] = true
		}
		r.Lines = append(r.Lines, *l)
	}
	sort.Slice(r.Lines, func(i, j int) bool { return r.Lines[i].SavedUSD > r.Lines[j].SavedUSD })
	for s := range unpriced {
		r.UnpricedSizes = append(r.UnpricedSizes, s)
	}
	sort.Strings(r.UnpricedSizes)

	window := to.Sub(from)
	r.SleepingPercent = round(100*union(asleep).Seconds()/window.Seconds(), 1)
	r.SleptDynoHours = round(r.SleptDynoHours, 2)
	r.SavedUSD = round(r.SavedUSD, 2)
	if days := window.Hours() / 24; days >= 1 {
		r.ProjectedMonthUSD = round(r.SavedUSD/days*30, 2)
	}
	return r
}

// union returns the total time covered by possibly overlapping intervals.
func union(ivs []Interval) time.Duration {
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].Start.Before(ivs[j].Start) })
	var total time.Duration
	var curS, curE time.Time
	for i, iv := range ivs {
		if i == 0 || iv.Start.After(curE) {
			total += curE.Sub(curS)
			curS, curE = iv.Start, iv.End
		} else if iv.End.After(curE) {
			curE = iv.End
		}
	}
	return total + curE.Sub(curS)
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// ForApp computes savings for one app from its stored history. The window
// never starts before the app was enrolled, so percentages aren't diluted.
func ForApp(db *gorm.DB, app *models.Application, from, to time.Time) Report {
	if app.CreatedAt.After(from) {
		from = app.CreatedAt
	}
	var events []models.AppHistory
	// Include earlier events so a sleep spanning the window start counts.
	db.Where("application_id = ? AND performed_at <= ? AND performed_at >= ?", app.ID, to, from.AddDate(0, 0, -90)).
		Order("performed_at, id").Find(&events)
	return Compute(events, from, to)
}
