package models

import (
	"github.com/gomodule/redigo/redis"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
)

var DB *gorm.DB
var REDIS *redis.Pool

func ConnectDataBase() {
	// dsn := "user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"
	dsn := os.Getenv("DATABASE_URL")
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})

	if err != nil {
		panic(err)
		panic("Failed to connect to database!")
	}

	database.AutoMigrate(&Application{})
	database.AutoMigrate(&AppHistory{})
	database.AutoMigrate(&Account{})
	DB = database
}

func RedisConn() { //adding it to model package as it's already included in most places
	var conn = &redis.Pool{
		MaxActive: 5,
		MaxIdle:   5,
		Wait:      true,
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(os.Getenv("REDISCLOUD_URL"))
		},
	}
	REDIS = conn
}
