package main

import (
	"autosleep/controllers"
	"autosleep/models"
	"autosleep/workers"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gocraft/work"
	"os"
	"os/signal"
)

func main() {
	models.ConnectDataBase()
	models.RedisConn()
	handle_routes()
	handle_worker()

}

func handle_routes() {
	router := gin.Default()

	authorized := router.Group("/", gin.BasicAuth(gin.Accounts{
		os.Getenv("API_USER"): os.Getenv("API_PASS"),
	}))

	authorized.GET("/app", controllers.FindApplications)
	authorized.POST("/app", controllers.CreateApp)
	authorized.GET("/app/:app_id", controllers.FindApp)
	authorized.DELETE("/app/:app_id", controllers.DeleteApp)
	router.POST("/drain/:app_id", controllers.ProcessDrain)
	// router.GET("/apps/:app_id/history", controllers.FindApplications)

	authorized.GET("/register", controllers.HandleAuth)
	router.GET("/auth/heroku/callback", controllers.HandleAuthCallback)
	go router.Run()
}

func handle_worker() {

	var pool = work.NewWorkerPool(workers.Context{}, 10, "auto_ideal", models.REDIS)

	pool.Job("sleep_checker", (*workers.Context).SleepChecker)
	pool.Start()
	fmt.Println("Worker started")
	// Wait for a signal to quit:
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	<-signalChan

	// Stop the pool
	pool.Stop()
}
