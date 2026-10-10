package demo

import (
	"autosleep/models"
	"testing"
)

func TestSetupAndSeed(t *testing.T) {
	Setup()
	Seed("http://localhost:8080")
	var apps []models.Application
	models.DB.Find(&apps)
	if len(apps) != 3 {
		t.Fatalf("enrolled %d demo apps, want 3", len(apps))
	}
	var events int64
	models.DB.Model(&models.AppHistory{}).Count(&events)
	if events == 0 {
		t.Fatal("no history seeded, savings report would be empty")
	}
	for _, a := range apps {
		if a.DrainToken == "" || a.LastDrainAt == nil {
			t.Fatalf("%s not fully set up: %+v", a.HerokuAppName, a)
		}
	}
}
