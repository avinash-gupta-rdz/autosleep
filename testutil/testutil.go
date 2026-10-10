// Package testutil provides an in-memory database and a fake Heroku API for
// tests.
package testutil

import (
	"autosleep/herokuclient/fake"
	"autosleep/models"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB returns a migrated in-memory SQLite database with foreign keys
// enforced (like MySQL/Postgres) and installs it as models.DB.
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := models.Migrate(db); err != nil {
		t.Fatal(err)
	}
	prev := models.DB
	models.DB = db
	t.Cleanup(func() {
		models.DB = prev
		sqlDB.Close()
	})
	return db
}

// FakeHeroku is the in-memory Heroku API; see package fake.
type FakeHeroku = fake.Heroku

// ErrNotFound is what the fake returns for unknown drains.
var ErrNotFound = fake.ErrNotFound

// Scheduler records scheduled checks.
type Scheduler struct {
	mu     sync.Mutex
	Checks map[string]time.Duration
}

func (s *Scheduler) ScheduleCheck(app string, in time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Checks == nil {
		s.Checks = map[string]time.Duration{}
	}
	s.Checks[app] = in
	return nil
}

// Scheduled returns the delay of the last check scheduled for app.
func (s *Scheduler) Scheduled(app string) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Checks[app]
	return d, ok
}
