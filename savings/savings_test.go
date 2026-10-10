package savings

import (
	"autosleep/models"
	"testing"
	"time"
)

var day = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func ev(action bool, typ, size string, qty int, at time.Duration) models.AppHistory {
	return models.AppHistory{Action: action, DynoName: typ, DynoSize: size, Quantity: qty, PerformedAt: day.Add(at)}
}

func TestComputeSleepWakePairs(t *testing.T) {
	events := []models.AppHistory{
		// Two standard-1X web dynos + one worker sleep 8 hours overnight.
		ev(models.ActionSleep, "web", "Standard-1X", 2, 0),
		ev(models.ActionSleep, "worker", "Standard-2X", 1, 0),
		ev(models.ActionWake, "web", "Standard-1X", 2, 8*time.Hour),
		ev(models.ActionWake, "worker", "Standard-2X", 1, 8*time.Hour),
	}
	r := Compute(events, day, day.Add(24*time.Hour))
	if r.SleptDynoHours != 24 {
		t.Fatalf("slept dyno-hours = %v, want 24 (2*8 + 1*8)", r.SleptDynoHours)
	}
	want := 16*0.035 + 8*0.070
	if r.SavedUSD != round(want, 2) {
		t.Fatalf("saved = %v, want %v", r.SavedUSD, round(want, 2))
	}
	if r.SleepingPercent != 33.3 {
		t.Fatalf("sleeping%% = %v, want 33.3", r.SleepingPercent)
	}
	if r.ProjectedMonthUSD != round(r.SavedUSD*30, 2) {
		t.Fatalf("projection = %v", r.ProjectedMonthUSD)
	}
}

func TestOpenIntervalRunsToWindowEnd(t *testing.T) {
	events := []models.AppHistory{ev(models.ActionSleep, "web", "Basic", 1, 20*time.Hour)}
	r := Compute(events, day, day.Add(24*time.Hour))
	if r.SleptDynoHours != 4 {
		t.Fatalf("slept = %v, want 4", r.SleptDynoHours)
	}
}

func TestWindowClipsIntervals(t *testing.T) {
	events := []models.AppHistory{
		ev(models.ActionSleep, "web", "Standard-1X", 1, -10*time.Hour), // started before window
		ev(models.ActionWake, "web", "Standard-1X", 1, 2*time.Hour),
	}
	r := Compute(events, day, day.Add(24*time.Hour))
	if r.SleptDynoHours != 2 {
		t.Fatalf("slept = %v, want 2", r.SleptDynoHours)
	}
}

func TestUnknownSizeIsFlagged(t *testing.T) {
	events := []models.AppHistory{
		ev(models.ActionSleep, "web", "Mystery-9X", 1, 0),
		ev(models.ActionWake, "web", "Mystery-9X", 1, time.Hour),
	}
	r := Compute(events, day, day.Add(24*time.Hour))
	if len(r.UnpricedSizes) != 1 || r.UnpricedSizes[0] != "Mystery-9X" || r.SavedUSD != 0 {
		t.Fatalf("unexpected report %+v", r)
	}
}

func TestPriceOverrides(t *testing.T) {
	t.Setenv("DYNO_HOURLY_PRICES", "mystery-9x=1.5, Standard-1X = 0.04")
	defer func() { HourlyPrice["standard-1x"] = 0.035; delete(HourlyPrice, "mystery-9x") }()
	LoadPriceOverrides()
	if p, ok := Price("Mystery-9X"); !ok || p != 1.5 {
		t.Fatalf("override not applied: %v %v", p, ok)
	}
	if p, _ := Price("standard-1X"); p != 0.04 {
		t.Fatalf("override with spaces not applied: %v", p)
	}
}

func TestRepeatedSleepWithoutWake(t *testing.T) {
	events := []models.AppHistory{
		ev(models.ActionSleep, "web", "Standard-1X", 1, 0),
		ev(models.ActionSleep, "web", "Standard-1X", 1, 3*time.Hour),
		ev(models.ActionWake, "web", "Standard-1X", 1, 5*time.Hour),
	}
	if got := Compute(events, day, day.Add(24*time.Hour)).SleptDynoHours; got != 5 {
		t.Fatalf("slept = %v, want 5", got)
	}
}
