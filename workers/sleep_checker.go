package workers

import (
	"autosleep/constants"
	"autosleep/models"
	"context"
	"fmt"
	"github.com/gocraft/work"
	heroku "github.com/heroku/heroku-go/v5"
	"strconv"
	"time"
)

type Context struct{}

func (c *Context) SleepChecker(job *work.Job) error {
	app_id := job.ArgString("app_id")
	var app models.Application

	if err := models.DB.Preload("Account").Where("heroku_app_name = ?", app_id).First(&app).Error; err != nil {
		return nil
	}
	loc, _ := time.LoadLocation("UTC")
	current := time.Now().In(loc)

	fmt.Println("SleepChecker: Current time is ", current, "dyno was active at ", app.RecentActivityAt)
	diff := current.Sub(app.RecentActivityAt)
	if diff.Seconds() > app.IdealTime && app.CurrentStatus == true {

		if app.NightMode && !CheckForNight() {
			var enqueuer = work.NewEnqueuer("auto_ideal", models.REDIS)
			_, err := enqueuer.EnqueueUniqueIn("sleep_checker", app.CheckInterval, work.Q{"app_id": app.HerokuAppName})
			if err != nil {
				fmt.Println(err)
			}
		} else {
			formation_list := ScaleDownDynos(app)
			models.DB.Debug().Model(&app).Select("CurrentStatus", "CurrentConfig").Updates(models.Application{CurrentStatus: false, CurrentConfig: formation_list})
		}

	}
	if diff.Seconds() <= app.IdealTime && app.CurrentStatus == true {
		var enqueuer = work.NewEnqueuer("auto_ideal", models.REDIS)
		_, err := enqueuer.EnqueueUniqueIn("sleep_checker", app.CheckInterval, work.Q{"app_id": app.HerokuAppName})
		if err != nil {
			fmt.Println(err)
		}
	}
	return nil
}

func ScaleDownDynos(app models.Application) map[string]map[string]string {
	heroku.DefaultTransport.BearerToken = app.GetToken()
	service := heroku.NewService(heroku.DefaultClient)
	var lrange heroku.ListRange

	formation_list, err := service.FormationList(context.TODO(), app.HerokuAppName, &lrange)
	if err != nil {
		fmt.Println(err)
	}

	opts := heroku.FormationBatchUpdateOpts{}
	var formation_map = map[string]map[string]string{}
	for _, formation := range formation_list {
		if formation.Quantity > 0 && formation.Type != "console" && formation.Type != "rake" {
			fmt.Println("formation ScaleDownDyno: ", formation.Type)
			qty := 0
			//build update struct
			upd := struct {
				Quantity *int    `json:"quantity,omitempty" url:"quantity,omitempty,key"`
				Size     *string `json:"size,omitempty" url:"size,omitempty,key"`
				Type     string  `json:"type" url:"type,key"`
			}{&qty, nil, formation.Type}
			opts.Updates = append(opts.Updates, upd)
		}

		if formation.Type != "console" && formation.Type != "rake" {
			formation_map[formation.Type] = map[string]string{}
			formation_map[formation.Type]["Quantity"] = strconv.Itoa(formation.Quantity)
			formation_map[formation.Type]["Size"] = formation.Size
		}

	}
	service.FormationBatchUpdate(context.TODO(), app.HerokuAppName, opts)
	return formation_map
}

func CheckForNight() bool {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)
	h, m, d := now.Date()
	start_time := time.Date(h, m, d, constants.NightModeStart["hour"], constants.NightModeStart["minute"], now.Second(), now.Nanosecond(), time.UTC)
	end_time := start_time.Add(constants.NightModeHour)
	if now.After(start_time) && now.Before(end_time) {
		return true
	} else {
		return false
	}
}
