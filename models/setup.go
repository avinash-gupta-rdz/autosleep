package models

import (
  "github.com/jinzhu/gorm"
  "github.com/gomodule/redigo/redis"
  _ "github.com/jinzhu/gorm/dialects/mysql"
  "os"
)

var DB *gorm.DB
var REDIS *redis.Pool
func ConnectDataBase() {
  database, err := gorm.Open("mysql", os.Getenv("DATABASE_URL"))

  if err != nil {
  	panic(err)
    panic("Failed to connect to database!")
  }

  database.AutoMigrate(&Application{})
  database.AutoMigrate(&AppHistory{})
  DB = database
}

func RedisConn(){ //adding it to model package as it's already included in most places
  var conn = &redis.Pool{
    MaxActive: 5,
    MaxIdle: 5,
    Wait: true,
    Dial: func() (redis.Conn, error) {
      return redis.DialURL(os.Getenv("REDISCLOUD_URL"))
    },
  }
  REDIS = conn
}

