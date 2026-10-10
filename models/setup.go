package models

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB
var REDIS *redis.Pool

// databaseURL returns the first database URL found among the env vars set
// by common Heroku add-ons.
func databaseURL() string {
	for _, k := range []string{"DATABASE_URL", "JAWSDB_URL", "JAWSDB_MARIA_URL", "CLEARDB_DATABASE_URL"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// Dialector picks the gorm driver for a database URL. It accepts Postgres
// URLs (postgres://), MySQL URLs (mysql://user:pass@host:3306/db, as given
// by JawsDB/ClearDB) and raw go-sql-driver DSNs (user:pass@tcp(host)/db).
func Dialector(raw string) (gorm.Dialector, error) {
	switch {
	case raw == "":
		return nil, fmt.Errorf("DATABASE_URL is not set")
	case strings.HasPrefix(raw, "postgres://"), strings.HasPrefix(raw, "postgresql://"):
		return postgres.Open(raw), nil
	case strings.HasPrefix(raw, "mysql://"):
		dsn, err := MySQLDSN(raw)
		if err != nil {
			return nil, err
		}
		return mysql.Open(dsn), nil
	default:
		return mysql.Open(raw), nil
	}
}

// MySQLDSN converts mysql://user:pass@host:port/db?opts into a
// go-sql-driver DSN, making sure times are parsed.
func MySQLDSN(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid MySQL URL: %w", err)
	}
	host := u.Host
	if u.Port() == "" {
		host += ":3306"
	}
	q := u.Query()
	q.Del("reconnect") // ClearDB adds this; go-sql-driver rejects it
	if q.Get("parseTime") == "" {
		q.Set("parseTime", "true")
	}
	if q.Get("charset") == "" {
		q.Set("charset", "utf8mb4")
	}
	userinfo := ""
	if u.User != nil {
		userinfo = u.User.Username()
		if p, ok := u.User.Password(); ok {
			userinfo += ":" + p
		}
		userinfo += "@"
	}
	return fmt.Sprintf("%stcp(%s)%s?%s", userinfo, host, u.Path, q.Encode()), nil
}

func ConnectDataBase() {
	dialector, err := Dialector(databaseURL())
	if err != nil {
		log.Fatal(err)
	}
	database, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if sqlDB, err := database.DB(); err == nil {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
	}
	if err := Migrate(database); err != nil {
		log.Fatalf("database migration failed: %v", err)
	}
	DB = database
}

// Migrate creates or updates all tables.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Account{}, &Application{}, &AppHistory{}, &EnrollOptOut{})
}

func redisURL() string {
	for _, k := range []string{"REDISCLOUD_URL", "REDIS_URL", "REDIS_TLS_URL"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "redis://127.0.0.1:6379"
}

// RedisConn sets up the Redis pool used by the job queue.
func RedisConn() {
	raw := redisURL()
	opts := []redis.DialOption{redis.DialConnectTimeout(10 * time.Second)}
	if strings.HasPrefix(raw, "rediss://") {
		// Heroku Key-Value Store uses self-signed certificates.
		opts = append(opts, redis.DialTLSConfig(&tls.Config{InsecureSkipVerify: true}))
	}
	REDIS = &redis.Pool{
		MaxActive:   10,
		MaxIdle:     5,
		IdleTimeout: 4 * time.Minute,
		Wait:        true,
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(raw, opts...)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}
