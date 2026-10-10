// Package schedule implements the daily "night mode" window during which
// night-mode apps are allowed to sleep.
package schedule

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // so NIGHT_MODE_TZ works on slim images
)

// Window is a daily time range starting at Hour:Minute in Location and
// lasting Duration. It may cross midnight.
type Window struct {
	Hour, Minute int
	Duration     time.Duration
	Location     *time.Location
}

// Contains reports whether t falls inside the window on any day.
func (w Window) Contains(t time.Time) bool {
	t = t.In(w.Location)
	y, m, d := t.Date()
	start := time.Date(y, m, d, w.Hour, w.Minute, 0, 0, w.Location)
	// A window that started yesterday may still be running.
	for _, s := range []time.Time{start, start.AddDate(0, 0, -1)} {
		if !t.Before(s) && t.Before(s.Add(w.Duration)) {
			return true
		}
	}
	return false
}

// String renders the window like "21:00 Asia/Kolkata for 12h0m0s".
func (w Window) String() string {
	return fmt.Sprintf("%02d:%02d %s for %s", w.Hour, w.Minute, w.Location, w.Duration)
}

// Default is 21:00 IST (15:30 UTC) for 12 hours, the historical behaviour.
var Default = Window{Hour: 15, Minute: 30, Duration: 12 * time.Hour, Location: time.UTC}

// FromEnv reads NIGHT_MODE_START ("HH:MM"), NIGHT_MODE_HOURS and
// NIGHT_MODE_TZ (IANA name, default UTC), falling back to Default.
func FromEnv() (Window, error) {
	w := Default
	if tz := os.Getenv("NIGHT_MODE_TZ"); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return Default, fmt.Errorf("NIGHT_MODE_TZ: %w", err)
		}
		w.Location = loc
	}
	if s := os.Getenv("NIGHT_MODE_START"); s != "" {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return Default, fmt.Errorf("NIGHT_MODE_START must be HH:MM, got %q", s)
		}
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return Default, fmt.Errorf("NIGHT_MODE_START must be HH:MM, got %q", s)
		}
		w.Hour, w.Minute = h, m
	}
	if s := os.Getenv("NIGHT_MODE_HOURS"); s != "" {
		h, err := strconv.ParseFloat(s, 64)
		if err != nil || h <= 0 || h > 24 {
			return Default, fmt.Errorf("NIGHT_MODE_HOURS must be between 0 and 24, got %q", s)
		}
		w.Duration = time.Duration(h * float64(time.Hour))
	}
	return w, nil
}
