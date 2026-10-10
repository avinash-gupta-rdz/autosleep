package workers

import (
	"autosleep/herokuclient"
	"autosleep/metrics"
	"autosleep/models"
	"autosleep/sleeper"
	"context"
	"log"
	"time"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
	"gorm.io/gorm"
)

const (
	Namespace     = "auto_ideal"
	SleepCheckJob = "sleep_checker"
	ReconcileJob  = "reconcile"
	ReconcileSpec = "0 */5 * * * *" // every 5 minutes (seconds field first)
	jobTimeout    = 2 * time.Minute
	overdueSlack  = 2 // check intervals past the idle deadline before forcing a check
)

type Context struct{}

// SleepChecker puts an idle app to sleep or schedules its next check.
func (c *Context) SleepChecker(job *work.Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()
	return metrics.Job("sleep_checker", sleeper.Check(ctx, models.DB, job.ArgString("app_id")))
}

// Reconcile is a periodic safety net: it makes sure every awake app has a
// pending sleep check (job chains can be lost if Redis is flushed or a
// dyno dies mid-job) and backfills drain tokens for apps added before
// drain authentication existed.
func (c *Context) Reconcile(job *work.Job) error {
	// Separate deadlines so slow enrolments can't starve the safety net.
	enrollCtx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	sleeper.AutoEnroll(enrollCtx, models.DB)
	cancel()
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()
	Reconcile(ctx, models.DB, Queue{Pool: models.REDIS})
	return metrics.Job("reconcile", nil)
}

// Enqueuer is the part of the job queue Reconcile needs.
type Enqueuer interface {
	sleeper.Scheduler
	CheckNow(appName string) error
}

func Reconcile(ctx context.Context, db *gorm.DB, q Enqueuer) {
	var apps []models.Application
	if err := db.Preload("Account").Find(&apps).Error; err != nil {
		log.Printf("reconcile: %v", err)
		return
	}
	now := sleeper.Now()
	for i := range apps {
		app := &apps[i]
		if app.DrainId != "" && app.DrainToken == "" {
			backfillDrainToken(ctx, db, app)
		}
		if !app.CurrentStatus {
			if probeDue(app.ID, now) {
				forgetIfDeleted(ctx, db, app)
			}
			continue
		}
		if app.Paused {
			continue
		}
		deadline := app.RecentActivityAt.Add(app.IdleDuration() + overdueSlack*app.CheckEvery())
		overdue := now.After(deadline) && (!app.NightMode || sleeper.Night.Contains(now))
		var err error
		if overdue {
			// The unique-job lock may be stale; force a check.
			err = q.CheckNow(app.HerokuAppName)
		} else {
			err = q.ScheduleCheck(app.HerokuAppName, sleeper.NextCheckIn(app, now))
		}
		if err != nil {
			log.Printf("reconcile: %s: %v", app.HerokuAppName, err)
		}
	}
}

func backfillDrainToken(ctx context.Context, db *gorm.DB, app *models.Application) {
	token, err := app.GetToken()
	if err != nil {
		return
	}
	d, err := herokuclient.New(token).DrainInfo(ctx, app.HerokuAppName, app.DrainId)
	if err != nil || d.Token == "" {
		return
	}
	db.Model(&models.Application{}).Where("id = ?", app.ID).Update("drain_token", d.Token)
	app.DrainToken = d.Token
}

// Queue schedules sleep checks on the gocraft/work Redis queue.
type Queue struct {
	Pool *redis.Pool
}

func (q Queue) enqueuer() *work.Enqueuer { return work.NewEnqueuer(Namespace, q.Pool) }

// ScheduleCheck queues a check unless one is already pending for the app.
func (q Queue) ScheduleCheck(appName string, in time.Duration) error {
	_, err := q.enqueuer().EnqueueUniqueIn(SleepCheckJob, int64(in.Seconds()), work.Q{"app_id": appName})
	return err
}

// CheckNow queues an immediate check even if one is pending. Checks are
// idempotent, so a duplicate is harmless.
func (q Queue) CheckNow(appName string) error {
	_, err := q.enqueuer().Enqueue(SleepCheckJob, work.Q{"app_id": appName})
	return err
}

// NewPool builds the worker pool with all jobs registered.
func NewPool(pool *redis.Pool, concurrency uint) *work.WorkerPool {
	wp := work.NewWorkerPool(Context{}, concurrency, Namespace, pool)
	wp.JobWithOptions(SleepCheckJob, work.JobOptions{MaxFails: 3}, (*Context).SleepChecker)
	wp.JobWithOptions(ReconcileJob, work.JobOptions{MaxFails: 1}, (*Context).Reconcile)
	wp.PeriodicallyEnqueue(ReconcileSpec, ReconcileJob)
	return wp
}

// probeDue spreads existence checks of sleeping apps so each one is probed
// about once an hour (every 12th five-minute reconcile).
func probeDue(appID uint, now time.Time) bool {
	return (int64(appID)+now.Unix()/300)%12 == 0
}

// forgetIfDeleted drops a sleeping app whose Heroku app no longer exists
// (typically a review app deleted after its PR closed). Awake apps are
// handled by the sleep checker.
func forgetIfDeleted(ctx context.Context, db *gorm.DB, app *models.Application) {
	token, err := app.GetToken()
	if err != nil {
		return
	}
	_, err = herokuclient.New(token).ListFormations(ctx, app.HerokuAppName)
	if herokuclient.IsNotFound(err) {
		log.Printf("reconcile: %s no longer exists on Heroku, forgetting it", app.HerokuAppName)
		sleeper.Forget(db, app)
	}
}
