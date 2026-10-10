package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Weekly is a per-app "office hours" window: on the selected weekdays it
// runs from From to To (minutes after midnight) in Location. When To is not
// after From the window ends on the next day, e.g. 22:00-06:00.
type Weekly struct {
	Days     [7]bool // indexed by time.Weekday
	From, To int
	Location *time.Location
}

var dayNames = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// ParseWeekly builds a window from API fields. days is a comma separated
// list of weekdays or ranges ("mon-fri", "mon,wed,fri", "sat-sun"); from and
// to are "HH:MM"; tz is an IANA zone name (empty means UTC).
func ParseWeekly(days, from, to, tz string) (Weekly, error) {
	var w Weekly
	for _, part := range strings.Split(strings.ToLower(days), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, ok1 := dayIndex(lo)
		b, ok2 := a, ok1
		if isRange {
			b, ok2 = dayIndex(hi)
		}
		if !ok1 || !ok2 {
			return w, fmt.Errorf("unknown day %q (use mon, tue, wed, thu, fri, sat, sun)", part)
		}
		for i := a; ; i = (i + 1) % 7 {
			w.Days[i] = true
			if i == b {
				break
			}
		}
	}
	if w.Days == [7]bool{} {
		return w, fmt.Errorf("choose at least one day")
	}
	var err error
	if w.From, err = parseClock(from); err != nil {
		return w, err
	}
	if w.To, err = parseClock(to); err != nil {
		return w, err
	}
	if w.From == w.To {
		return w, fmt.Errorf("office hours must not start and end at the same time")
	}
	w.Location = time.UTC
	if tz != "" {
		if w.Location, err = time.LoadLocation(tz); err != nil {
			return w, fmt.Errorf("unknown time zone %q", tz)
		}
	}
	return w, nil
}

func dayIndex(s string) (int, bool) {
	s = strings.TrimSpace(s)
	for i, d := range dayNames {
		if len(s) >= 3 && strings.HasPrefix(s, d) {
			return i, true
		}
	}
	return 0, false
}

func parseClock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hh < 0 || hh > 24 || mm < 0 || mm > 59 || (hh == 24 && mm != 0) {
		return 0, fmt.Errorf("times must be HH:MM, got %q", s)
	}
	return hh*60 + mm, nil
}

// length is how long each daily window lasts.
func (w Weekly) length() time.Duration {
	d := w.To - w.From
	if d <= 0 {
		d += 24 * 60
	}
	return time.Duration(d) * time.Minute
}

// startOn returns the window start on the calendar day of t.
func (w Weekly) startOn(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, w.Location).Add(time.Duration(w.From) * time.Minute)
}

// Current returns the start and end of the window containing t, if any.
func (w Weekly) Current(t time.Time) (start, end time.Time, ok bool) {
	t = t.In(w.Location)
	// A window that started yesterday may still be running.
	for _, day := range []time.Time{t, t.AddDate(0, 0, -1)} {
		if !w.Days[day.Weekday()] {
			continue
		}
		s := w.startOn(day)
		e := s.Add(w.length())
		if !t.Before(s) && t.Before(e) {
			return s, e, true
		}
	}
	return time.Time{}, time.Time{}, false
}

// Contains reports whether t falls inside the window.
func (w Weekly) Contains(t time.Time) bool {
	_, _, ok := w.Current(t)
	return ok
}

// NextStart returns the first window start after t (within a week).
func (w Weekly) NextStart(t time.Time) time.Time {
	t = t.In(w.Location)
	for i := 0; i <= 7; i++ {
		day := t.AddDate(0, 0, i)
		if s := w.startOn(day); w.Days[day.Weekday()] && s.After(t) {
			return s
		}
	}
	return time.Time{}
}

// String renders the window like "mon-fri 09:00-18:00 Europe/Berlin".
func (w Weekly) String() string {
	var on []string
	for i, d := range dayNames {
		if w.Days[i] {
			on = append(on, d)
		}
	}
	return fmt.Sprintf("%s %02d:%02d-%02d:%02d %s", strings.Join(on, ","), w.From/60, w.From%60, w.To/60, w.To%60, w.Location)
}
