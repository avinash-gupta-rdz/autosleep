package schedule

import (
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 10, 7, h, m, 0, 0, time.UTC) }

func TestDefaultWindowCrossesMidnight(t *testing.T) {
	cases := []struct {
		t    time.Time
		want bool
	}{
		{at(15, 29), false},
		{at(15, 30), true},
		{at(23, 59), true},
		{at(0, 0), true}, // started yesterday
		{at(3, 29), true},
		{at(3, 30), false},
		{at(12, 0), false},
	}
	for _, c := range cases {
		if got := Default.Contains(c.t); got != c.want {
			t.Errorf("Contains(%s) = %v, want %v", c.t.Format("15:04"), got, c.want)
		}
	}
}

func TestWindowInTimezone(t *testing.T) {
	t.Setenv("NIGHT_MODE_TZ", "Asia/Kolkata")
	t.Setenv("NIGHT_MODE_START", "21:00")
	t.Setenv("NIGHT_MODE_HOURS", "10")
	w, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	// 21:00 IST == 15:30 UTC; window ends 07:00 IST == 01:30 UTC.
	if !w.Contains(at(15, 30)) || !w.Contains(at(1, 29)) {
		t.Error("expected inside window")
	}
	if w.Contains(at(1, 30)) || w.Contains(at(15, 29)) {
		t.Error("expected outside window")
	}
}

func TestFromEnvRejectsBadValues(t *testing.T) {
	for _, v := range []string{"25:00", "9", "ab:cd"} {
		t.Setenv("NIGHT_MODE_START", v)
		if _, err := FromEnv(); err == nil {
			t.Errorf("NIGHT_MODE_START=%q: expected error", v)
		}
	}
}
