package controllers

import (
	"autosleep/constants"
	"autosleep/models"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gocraft/work"
	"net/http"
	"strings"
	"time"
)

func FindApplications(c *gin.Context) {
	var applications []models.Application
	models.DB.Select("ID, heroku_app_name, current_status, recent_activity_at, drain_id").Find(&applications)

	c.JSON(http.StatusOK, gin.H{"list": applications})
}

type CreateApplicationInput struct {
	HerokuAppName string  `json:"heroku_app_name" binding:"required"`
	HerokuApiKey  string  `json:"heroku_api_key"`
	CheckInterval int64   `json:"check_interval"`
	IdealTime     float64 `json:"ideal_time"`
	ManualMode    bool    `json:"manual_mode"`
	NightMode     bool    `json:"night_mode"`
	AccountId     uint    `json:"account_id"`
}

func CreateApp(c *gin.Context) {
	// Validate input
	input := CreateApplicationInput{CheckInterval: constants.CheckInterval, IdealTime: constants.IdealTime, ManualMode: false, NightMode: false}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fmt.Println("acid ", input.AccountId)
	fmt.Println("key ", input.HerokuApiKey)
	if input.AccountId == 0 && input.HerokuApiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either API key or AccountId is required "})
		return
	}
	token := input.HerokuApiKey
	if input.AccountId != 0 {
		var act models.Account
		if err := models.DB.Where("id = ?", input.AccountId).First(&act).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
			return
		}
		token = act.CurrentAccessToken()
	}

	cur_config := get_current_config(input.HerokuAppName, token)
	drain_id := add_drain(input.HerokuAppName, token)
	var encrypted_tok []byte
	if input.HerokuApiKey != "" {
		encrypted_tok = models.Encrypt(token)
	}
	app := models.Application{HerokuAppName: input.HerokuAppName, HerokuApiKey: encrypted_tok, CheckInterval: input.CheckInterval, IdealTime: input.IdealTime, NightMode: input.NightMode, ManualMode: input.ManualMode, DrainId: drain_id, RecentActivityAt: time.Now(), CurrentConfig: cur_config, CurrentStatus: true, AccountId: input.AccountId}
	models.DB.Create(&app)
	var enqueuer = work.NewEnqueuer("auto_ideal", models.REDIS)
	_, err := enqueuer.EnqueueIn("sleep_checker", app.CheckInterval, work.Q{"app_id": input.HerokuAppName})
	if err != nil {
		fmt.Println(err)
	}
	c.JSON(http.StatusOK, gin.H{"data": app})
}

func ProcessDrain(c *gin.Context) {

	buf := make([]byte, 1024)
	num, _ := c.Request.Body.Read(buf)
	reqBody := string(buf[0:num])
	is_running := strings.Contains(reqBody, "router") && !strings.Contains(reqBody, "well-known")

	if is_running == false {
		c.JSON(http.StatusOK, gin.H{"status": is_running})
		return
	}
	var app models.Application
	if err := models.DB.Preload("Account").Where("heroku_app_name = ?", c.Param("app_id")).First(&app).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
		return
	}
	fmt.Println("app_id ", c.Param("app_id"), " CurrentStatus: ", app.CurrentStatus, " is_running: ", is_running)
	if app.CurrentStatus == false && is_running == true && app.ManualMode == false {
		ScaleUpDynos(app)
		models.DB.Model(&app).Update("CurrentStatus", true)
		var enqueuer = work.NewEnqueuer("auto_ideal", models.REDIS)
		_, err := enqueuer.EnqueueUniqueIn("sleep_checker", app.CheckInterval, work.Q{"app_id": app.HerokuAppName})
		if err != nil {
			fmt.Println(err)
		}
	}
	if is_running {
		models.DB.Model(&app).Update("RecentActivityAt", time.Now())
	}
	c.JSON(http.StatusOK, gin.H{"status": is_running})
}

func FindApp(c *gin.Context) {
	var app models.Application

	if err := models.DB.Preload("Account").Where("heroku_app_name = ?", c.Param("app_id")).First(&app).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": app})
}

func DeleteApp(c *gin.Context) {
	var app models.Application

	if err := models.DB.Preload("Account").Where("heroku_app_name = ?", c.Param("app_id")).First(&app).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
		return
	}
	remove_drain(app.HerokuAppName, app.GetToken(), app.DrainId)
	models.DB.Unscoped().Delete(&app)
	c.JSON(http.StatusOK, gin.H{"data": "The App is deleted"})
}
