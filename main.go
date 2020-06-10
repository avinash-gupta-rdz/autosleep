package main

import (
  // "net/http"
  "github.com/gin-gonic/gin"
  "autosleep/models"
  "autosleep/controllers"
  "autosleep/workers"
  "github.com/gocraft/work"
  "fmt"
  "os"
  "os/signal"
  // "log"
)


func main() {
  models.ConnectDataBase()
  models.RedisConn()
  handle_routes()
  handle_worker()
  
}

func handle_routes(){
  router := gin.Default()
  router.GET("/app", controllers.FindApplications)
  router.GET("/app/:app_id", controllers.FindApp)
  router.DELETE("/app/:app_id", controllers.DeleteApp)
  // router.GET("/apps/:app_id/history", controllers.FindApplications)
  router.POST("/app",controllers.CreateApp) //DONE

  router.POST("/drain/:app_id", controllers.ProcessDrain)
  go router.Run()
}

func handle_worker(){
  // Make a redis pool
  // var redisPool = models.REDISw
  var pool = work.NewWorkerPool(workers.Context{}, 10, "auto_ideal", models.REDIS)

  pool.Job("sleep_chacker", (*workers.Context).SleepChecker)
  pool.Start()
  fmt.Println("Worker started")
  // Wait for a signal to quit:
  signalChan := make(chan os.Signal, 1)
  signal.Notify(signalChan, os.Interrupt, os.Kill)
  <-signalChan

  // Stop the pool
  pool.Stop()
}