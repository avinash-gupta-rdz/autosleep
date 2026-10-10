package controllers

import (
	"autosleep/constants"
	"autosleep/herokuclient"
	"autosleep/models"
	"autosleep/savings"
	"autosleep/schedule"
	"autosleep/sleeper"
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const herokuTimeout = 30 * time.Second

func FindApplications(c *gin.Context) {
	var apps []models.Application
	if err := models.DB.Preload("Account").Order("heroku_app_name").Find(&apps).Error; err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	list := make([]AppView, 0, len(apps))
	for i := range apps {
		list = append(list, viewOf(&apps[i]))
	}
	c.JSON(http.StatusOK, gin.H{"list": list})
}

func FindApp(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": viewOf(app)})
}

type CreateApplicationInput struct {
	HerokuAppName string  `json:"heroku_app_name" binding:"required"`
	HerokuApiKey  string  `json:"heroku_api_key"`
	CheckInterval int64   `json:"check_interval"`
	IdealTime     float64 `json:"ideal_time"`
	ManualMode    bool    `json:"manual_mode"`
	NightMode     bool    `json:"night_mode"`
	WebOnly       bool    `json:"web_only"`
	AccountId     uint    `json:"account_id"`
	// WakePage defaults to on (see WAKE_PAGE_DEFAULT).
	WakePage *bool `json:"wake_page"`
	OfficeHoursInput
}

// OfficeHoursInput sets the keep-awake window, e.g. awake_days "mon-fri",
// awake_from "09:00", awake_to "18:00", awake_tz "Europe/Berlin". An empty
// awake_days removes it.
type OfficeHoursInput struct {
	AwakeDays *string `json:"awake_days"`
	AwakeFrom string  `json:"awake_from"`
	AwakeTo   string  `json:"awake_to"`
	AwakeTZ   string  `json:"awake_tz"`
}

// officeHours validates the input and returns the columns to store, or nil
// when the input doesn't mention office hours.
func (in OfficeHoursInput) officeHours() (map[string]interface{}, error) {
	if in.AwakeDays == nil {
		return nil, nil
	}
	days := strings.TrimSpace(*in.AwakeDays)
	if days == "" {
		return map[string]interface{}{"awake_days": "", "awake_from": "", "awake_to": "", "awake_tz": ""}, nil
	}
	tz := in.AwakeTZ
	if tz == "" {
		tz = sleeper.Night.Location.String()
	}
	w, err := schedule.ParseWeekly(days, in.AwakeFrom, in.AwakeTo, tz)
	if err != nil {
		return nil, fmt.Errorf("office hours: %w", err)
	}
	return map[string]interface{}{"awake_days": days, "awake_from": in.AwakeFrom, "awake_to": in.AwakeTo, "awake_tz": w.Location.String()}, nil
}

// Limits keep users from configuring an app that flaps or never sleeps.
const (
	minIdealTime     = 60
	minCheckInterval = 30
)

func validateTimes(ideal float64, interval int64) string {
	if ideal < minIdealTime {
		return fmt.Sprintf("ideal_time must be at least %d seconds", minIdealTime)
	}
	if interval < minCheckInterval {
		return fmt.Sprintf("check_interval must be at least %d seconds", minCheckInterval)
	}
	return ""
}

func CreateApp(c *gin.Context) {
	input := CreateApplicationInput{CheckInterval: constants.CheckInterval, IdealTime: constants.IdealTime}
	if err := c.ShouldBindJSON(&input); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	input.HerokuAppName = strings.TrimSpace(input.HerokuAppName)
	if input.AccountId == 0 && input.HerokuApiKey == "" {
		jsonError(c, http.StatusBadRequest, "Either heroku_api_key or account_id is required")
		return
	}
	if msg := validateTimes(input.IdealTime, input.CheckInterval); msg != "" {
		jsonError(c, http.StatusBadRequest, msg)
		return
	}
	hours, err := input.officeHours()
	if err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	var existing int64
	models.DB.Model(&models.Application{}).Where("heroku_app_name = ?", input.HerokuAppName).Count(&existing)
	if existing > 0 {
		jsonError(c, http.StatusConflict, "App "+input.HerokuAppName+" is already managed by AutoSleep")
		return
	}

	app := models.Application{
		HerokuAppName: input.HerokuAppName, CheckInterval: input.CheckInterval, IdealTime: input.IdealTime,
		NightMode: input.NightMode, ManualMode: input.ManualMode, WebOnly: input.WebOnly,
		RecentActivityAt: sleeper.Now(), CurrentStatus: true,
	}
	if hours != nil {
		app.AwakeDays, app.AwakeFrom = hours["awake_days"].(string), hours["awake_from"].(string)
		app.AwakeTo, app.AwakeTZ = hours["awake_to"].(string), hours["awake_tz"].(string)
	}
	if input.AccountId != 0 {
		if err := models.DB.First(&app.Account, input.AccountId).Error; err != nil {
			jsonError(c, http.StatusBadRequest, "Account not found; connect one via /register")
			return
		}
		app.AccountId = &app.Account.ID
	}
	if input.HerokuApiKey != "" {
		app.HerokuApiKey = models.Encrypt(input.HerokuApiKey)
	}
	if err := sleeper.Enroll(c.Request.Context(), models.DB, &app, BaseURL(c)); err != nil {
		jsonError(c, http.StatusBadGateway, err.Error())
		return
	}
	sleeper.OptIn(models.DB, app.HerokuAppName)
	sleeper.ScheduleNextCheck(&app)
	resp := gin.H{}
	if wantPage := sleeper.WakePageByDefault(); (input.WakePage == nil && wantPage) || (input.WakePage != nil && *input.WakePage) {
		if err := sleeper.EnableWakePageOnEnroll(c.Request.Context(), models.DB, &app, BaseURL(c)); err != nil {
			resp["warning"] = "App added, but the waking page wasn't enabled: " + err.Error()
		}
	}
	resp["data"] = viewOf(&app)
	c.JSON(http.StatusOK, resp)
}

// UpdateApplicationInput uses pointers so omitted fields are left alone.
type UpdateApplicationInput struct {
	CheckInterval *int64   `json:"check_interval"`
	IdealTime     *float64 `json:"ideal_time"`
	ManualMode    *bool    `json:"manual_mode"`
	NightMode     *bool    `json:"night_mode"`
	WebOnly       *bool    `json:"web_only"`
	Paused        *bool    `json:"paused"`
	WakePage      *bool    `json:"wake_page"`
	HerokuApiKey  *string  `json:"heroku_api_key"`
	OfficeHoursInput
}

func UpdateApp(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	var in UpdateApplicationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	ideal, interval := app.IdealTime, app.CheckInterval
	if in.IdealTime != nil {
		ideal = *in.IdealTime
	}
	if in.CheckInterval != nil {
		interval = *in.CheckInterval
	}
	if msg := validateTimes(ideal, interval); msg != "" {
		jsonError(c, http.StatusBadRequest, msg)
		return
	}
	updates := map[string]interface{}{"ideal_time": ideal, "check_interval": interval}
	hours, err := in.officeHours()
	if err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	for k, v := range hours {
		updates[k] = v
	}
	if in.ManualMode != nil {
		updates["manual_mode"] = *in.ManualMode
	}
	if in.NightMode != nil {
		updates["night_mode"] = *in.NightMode
	}
	if in.WebOnly != nil {
		updates["web_only"] = *in.WebOnly
	}
	if in.HerokuApiKey != nil && *in.HerokuApiKey != "" {
		updates["heroku_api_key"] = models.Encrypt(*in.HerokuApiKey)
	}
	if in.WakePage != nil && *in.WakePage != app.WakePage {
		ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
		defer cancel()
		if err := sleeper.SetWakePage(ctx, app, BaseURL(c), *in.WakePage); err != nil {
			jsonError(c, http.StatusBadGateway, err.Error())
			return
		}
		updates["wake_page"] = *in.WakePage
	}
	if err := models.DB.Model(&models.Application{}).Where("id = ?", app.ID).Updates(updates).Error; err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if in.Paused != nil && *in.Paused != app.Paused {
		if *in.Paused {
			pause(c, app.HerokuAppName)
		} else {
			resume(c, app.HerokuAppName)
		}
		return
	}
	if fresh, err := sleeper.Load(models.DB, app.HerokuAppName); err == nil {
		sleeper.ScheduleNextCheck(fresh)
	}
	respondApp(c, app.HerokuAppName)
}

// WakeApp scales an app back up now (works in manual mode too).
func WakeApp(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	if _, err := sleeper.Wake(ctx, models.DB, app, sleeper.ReasonManual); err != nil {
		jsonError(c, http.StatusBadGateway, err.Error())
		return
	}
	respondApp(c, app.HerokuAppName)
}

// SleepApp puts an app to sleep now, regardless of recent traffic.
func SleepApp(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	if app.Paused {
		jsonError(c, http.StatusConflict, "App is paused; resume it first")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	if _, err := sleeper.Sleep(ctx, models.DB, app, sleeper.ReasonManual, true); err != nil {
		jsonError(c, http.StatusBadGateway, err.Error())
		return
	}
	respondApp(c, app.HerokuAppName)
}

func PauseApp(c *gin.Context)  { pause(c, c.Param("app_id")) }
func ResumeApp(c *gin.Context) { resume(c, c.Param("app_id")) }

// pause disables AutoSleep for an app, waking it first if it is asleep.
func pause(c *gin.Context, name string) {
	app, err := sleeper.Load(models.DB, name)
	if err != nil {
		jsonError(c, http.StatusNotFound, "App not found")
		return
	}
	// Set the flag first: from then on no sleep can start (its CAS requires
	// paused = false), so waking afterwards leaves the app running.
	if err := models.DB.Model(&models.Application{}).Where("id = ?", app.ID).Update("paused", true).Error; err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	if app, err = sleeper.Load(models.DB, name); err == nil {
		if _, err := sleeper.Wake(ctx, models.DB, app, sleeper.ReasonManual); err != nil {
			jsonError(c, http.StatusBadGateway, "paused, but could not wake the app: "+err.Error())
			return
		}
	}
	respondApp(c, name)
}

// resume re-enables AutoSleep; the idle clock restarts now.
func resume(c *gin.Context, name string) {
	app, err := sleeper.Load(models.DB, name)
	if err != nil {
		jsonError(c, http.StatusNotFound, "App not found")
		return
	}
	err = models.DB.Model(&models.Application{}).Where("id = ?", app.ID).
		Updates(map[string]interface{}{"paused": false, "recent_activity_at": sleeper.Now()}).Error
	if err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if app, err = sleeper.Load(models.DB, name); err == nil {
		sleeper.ScheduleNextCheck(app)
	}
	respondApp(c, name)
}

// DeleteApp stops managing an app. A sleeping app is woken first so it is
// never left scaled to zero; pass ?force=true to skip that.
func DeleteApp(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	force := c.Query("force") == "true"

	// Pause first so no sleep can start while we remove the app, then wake
	// it from fresh state so it is never left scaled to zero.
	wasPaused := app.Paused
	models.DB.Model(&models.Application{}).Where("id = ?", app.ID).Update("paused", true)
	fail := func(msg string) {
		models.DB.Model(&models.Application{}).Where("id = ?", app.ID).Update("paused", wasPaused)
		jsonError(c, http.StatusBadGateway, msg+" (retry, or use ?force=true)")
	}
	if fresh, err := sleeper.Load(models.DB, app.HerokuAppName); err == nil {
		app = fresh
	}
	if !app.CurrentStatus {
		if _, err := sleeper.Wake(ctx, models.DB, app, sleeper.ReasonRemoved); err != nil && !force {
			fail("could not wake app before removing it: " + err.Error())
			return
		}
	}
	if app.WakePage {
		sleeper.SetWakePage(ctx, app, BaseURL(c), false)
	}
	if app.DrainId != "" {
		token, err := app.GetToken()
		if err == nil {
			err = herokuclient.New(token).DeleteDrain(ctx, app.HerokuAppName, app.DrainId)
		}
		if err != nil && !herokuclient.IsNotFound(err) && !force {
			fail("could not remove log drain: " + err.Error())
			return
		}
	}
	if err := sleeper.Forget(models.DB, app); err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Keep AUTO_ENROLL from adding it straight back.
	sleeper.OptOut(models.DB, app.HerokuAppName)
	c.JSON(http.StatusOK, gin.H{"data": "App " + app.HerokuAppName + " removed from AutoSleep"})
}

func AppHistory(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var events []models.AppHistory
	models.DB.Where("application_id = ?", app.ID).Order("performed_at desc, id desc").Limit(limit).Find(&events)
	c.JSON(http.StatusOK, gin.H{"list": events})
}

// savingsWindow parses ?days= (default 30, max 366).
func savingsWindow(c *gin.Context) (time.Time, time.Time) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || days <= 0 || days > 366 {
		days = 30
	}
	to := sleeper.Now()
	return to.AddDate(0, 0, -days), to
}

// appSavings computes savings for one app; the window never starts before
// the app was enrolled, so percentages aren't diluted.
func appSavings(app *models.Application, from, to time.Time) savings.Report {
	if app.CreatedAt.After(from) {
		from = app.CreatedAt
	}
	var events []models.AppHistory
	// Include earlier events so a sleep spanning the window start counts.
	models.DB.Where("application_id = ? AND performed_at <= ? AND performed_at >= ?", app.ID, to, from.AddDate(0, 0, -90)).
		Order("performed_at, id").Find(&events)
	return savings.Compute(events, from, to)
}

func AppSavings(c *gin.Context) {
	app, ok := loadApp(c)
	if !ok {
		return
	}
	from, to := savingsWindow(c)
	c.JSON(http.StatusOK, gin.H{"app": app.HerokuAppName, "data": appSavings(app, from, to)})
}

// Summary is the dashboard headline across all apps.
type Summary struct {
	Apps              int                       `json:"apps"`
	Asleep            int                       `json:"asleep"`
	Awake             int                       `json:"awake"`
	Paused            int                       `json:"paused"`
	SavedUSD          float64                   `json:"saved_usd"`
	ProjectedMonthUSD float64                   `json:"projected_monthly_usd"`
	SleptDynoHours    float64                   `json:"slept_dyno_hours"`
	Days              float64                   `json:"days"`
	UnpricedSizes     []string                  `json:"unpriced_sizes,omitempty"`
	PerApp            map[string]savings.Report `json:"per_app"`
}

func AllSavings(c *gin.Context) {
	from, to := savingsWindow(c)
	var apps []models.Application
	models.DB.Find(&apps)
	s := Summary{Days: to.Sub(from).Hours() / 24, PerApp: map[string]savings.Report{}}
	unpriced := map[string]bool{}
	for i := range apps {
		app := &apps[i]
		s.Apps++
		switch {
		case app.Paused:
			s.Paused++
		case app.CurrentStatus:
			s.Awake++
		default:
			s.Asleep++
		}
		r := appSavings(app, from, to)
		s.PerApp[app.HerokuAppName] = r
		s.SavedUSD += r.SavedUSD
		s.ProjectedMonthUSD += r.ProjectedMonthUSD
		s.SleptDynoHours += r.SleptDynoHours
		for _, u := range r.UnpricedSizes {
			if !unpriced[u] {
				unpriced[u] = true
				s.UnpricedSizes = append(s.UnpricedSizes, u)
			}
		}
	}
	s.SavedUSD = math.Round(s.SavedUSD*100) / 100
	s.ProjectedMonthUSD = math.Round(s.ProjectedMonthUSD*100) / 100
	s.SleptDynoHours = math.Round(s.SleptDynoHours*100) / 100
	c.JSON(http.StatusOK, gin.H{"data": s})
}
