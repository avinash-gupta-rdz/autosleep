package schedule

import (
	"testing"
	"time"
)

func TestWeeklyOfficeHours(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	w, err := ParseWeekly("mon-fri", "09:00", "18:00", "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	at := func(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, berlin) } // Oct 5 2026 is a Monday
	cases := []struct {
		t    time.Time
		want bool
	}{
		{at(5, 8, 59), false}, {at(5, 9, 0), true}, {at(9, 17, 59), true}, {at(9, 18, 0), false},
		{at(10, 12, 0), false}, // Saturday
	}
	for _, c := range cases {
		if got := w.Contains(c.t.UTC()); got != c.want {
			t.Errorf("Contains(%v) = %v", c.t, got)
		}
	}
	if got := w.NextStart(at(9, 19, 0)); !got.Equal(at(12, 9, 0)) {
		t.Errorf("NextStart Friday evening = %v, want Monday 09:00", got)
	}
	s, e, ok := w.Current(at(6, 10, 0))
	if !ok || !s.Equal(at(6, 9, 0)) || !e.Equal(at(6, 18, 0)) {
		t.Errorf("Current = %v %v %v", s, e, ok)
	}
}

func TestWeeklyOvernightAndParsing(t *testing.T) {
	w, err := ParseWeekly("fri", "22:00", "02:00", "")
	if err != nil {
		t.Fatal(err)
	}
	sat1am := time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)
	if !w.Contains(sat1am) {
		t.Error("Friday night window should run into Saturday")
	}
	if w.Contains(sat1am.Add(24 * time.Hour)) {
		t.Error("Saturday night is not in a Friday-only window")
	}
	if w, err := ParseWeekly("sat-sun", "10:00", "12:00", ""); err != nil || !w.Days[time.Sunday] || w.Days[time.Monday] {
		t.Errorf("sat-sun = %v, %v", w.Days, err)
	}
	for _, bad := range [][4]string{{"", "09:00", "18:00", ""}, {"funday", "09:00", "18:00", ""}, {"mon", "9", "18:00", ""}, {"mon", "09:00", "09:00", ""}, {"mon", "09:00", "18:00", "Mars/Base"}} {
		if _, err := ParseWeekly(bad[0], bad[1], bad[2], bad[3]); err == nil {
			t.Errorf("ParseWeekly%v accepted", bad)
		}
	}
}
