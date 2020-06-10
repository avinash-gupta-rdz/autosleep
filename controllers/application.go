package controllers

import (
  "github.com/gin-gonic/gin"
  "autosleep/models"
  "autosleep/constants"
  "net/http"
  "fmt"
  "time"
  "strings"
  "github.com/gocraft/work"
)

func FindApplications(c *gin.Context) {
  var applications []models.Application
  models.DB.Select("ID, heroku_app_name, current_status, recent_activity_at, drain_id").Find(&applications)

  c.JSON(http.StatusOK, gin.H{"list": applications})
}
//////////
type CreateApplicationInput struct {
  HerokuAppName  	string  `json:"heroku_app_name" binding:"required"`
  IncludeWorker 	bool 	`json:"include_worker"  binding:"required"`
  HerokuApiKey 		string	`json:"heroku_api_key"  binding:"required"`
}

func CreateApp(c *gin.Context) {
  // Validate input
  var input CreateApplicationInput
  if err := c.ShouldBindJSON(&input); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }
  
  cur_config := get_current_config(input.HerokuAppName, input.HerokuApiKey)
  drain_id := add_drain(input.HerokuAppName, input.HerokuApiKey)
  app := models.Application{HerokuAppName: input.HerokuAppName, HerokuApiKey: input.HerokuApiKey, IncludeWorker: input.IncludeWorker,DrainId: drain_id, RecentActivityAt: time.Now(),CurrentConfig: cur_config, CurrentStatus: true }
  models.DB.Create(&app)
  var enqueuer = work.NewEnqueuer("auto_ideal", models.REDIS)
  _, err := enqueuer.EnqueueIn("sleep_chacker",constants.CheckInterval,work.Q{"app_id": input.HerokuAppName})
  if err != nil {
    fmt.Println(err)
  }
  c.JSON(http.StatusOK, gin.H{"data": app})
}


func ProcessDrain(c *gin.Context){

  buf := make([]byte, 1024)
  num, _ := c.Request.Body.Read(buf)
  reqBody := string(buf[0:num])
  is_running := strings.Contains(reqBody, "router");
  if is_running == false {
    c.JSON(http.StatusOK, gin.H{"status": is_running})
    return
  }
  var app models.Application
  if err := models.DB.Where("heroku_app_name = ?", c.Param("app_id")).First(&app).Error; err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
    return
  }
  fmt.Println("app_id ", c.Param("app_id"), " CurrentStatus: ", app.CurrentStatus," is_running: ", is_running)
  if app.CurrentStatus == false &&  is_running == true {
    ScaleUpDynos(app)
    models.DB.Model(&app).Update("CurrentStatus", true)
    var enqueuer = work.NewEnqueuer("auto_ideal", models.REDIS)
    _, err := enqueuer.EnqueueUniqueIn("sleep_chacker",30,work.Q{"app_id": app.HerokuAppName})
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

  if err := models.DB.Where("heroku_app_name = ?", c.Param("app_id")).First(&app).Error; err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
    return
  }

  c.JSON(http.StatusOK, gin.H{"data": app})
}


func DeleteApp(c *gin.Context) {  
  var app models.Application

  if err := models.DB.Where("heroku_app_name = ?", c.Param("app_id")).First(&app).Error; err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "Record not found!"})
    return
  }
  remove_drain(app.HerokuAppName, app.HerokuApiKey, app.DrainId)
  models.DB.Delete(&app)
  c.JSON(http.StatusOK, gin.H{"data": "The App is deleted softly"})
}