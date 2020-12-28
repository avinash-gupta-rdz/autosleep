package models

import (
  "github.com/jinzhu/gorm"
  "time"
  "encoding/json"
  "database/sql/driver"
  "crypto/aes"
  "crypto/cipher"
  "crypto/md5"
  "crypto/rand"
  "encoding/hex"
  "io"
  "os"
)
type JSONB  map[string]map[string]string// map[string]interface{}

type Application struct {
  gorm.Model
  // ID     			uint   `json:"id" gorm:"primary_key"`
  HerokuAppName  	  string    `json:"heroku_app_name",sql:"unique_index"`
  CurrentStatus 	  bool      `json:"current_status"`
  RecentActivityAt  time.Time `json:"recent_activity_at"`
  HerokuApiKey 		  []byte      `json:"heroku_api_key"`
  DrainId 		      string    `json:"drain_id"`
  CurrentConfig     JSONB     `json:"current_config",sql:"type:jsonb"`
  IdealTime         float64   `json:"ideal_time"`
  CheckInterval     int64     `json:"check_interval"`
  ManualMode        bool      `json:"manual_mode"`
  NightMode         bool      `json:"night_mode"`


}

 func (j JSONB) Value() (driver.Value, error) {
  valueString, err := json.Marshal(j)
  return string(valueString), err
 }
 
 func (j *JSONB) Scan(value interface{}) error {
  if err := json.Unmarshal(value.([]byte), &j); err != nil {
    return err
  }
  return nil
 }

 func createHash(key string) string {
  hasher := md5.New()
  hasher.Write([]byte(key))
  return hex.EncodeToString(hasher.Sum(nil))
}

func Encrypt(data string) []byte {

  passphrase := os.Getenv("PASSPHRASE")
  block, _ := aes.NewCipher([]byte(createHash(passphrase)))
  gcm, err := cipher.NewGCM(block)
  if err != nil {
    panic(err.Error())
  }
  nonce := make([]byte, gcm.NonceSize())
  if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
    panic(err.Error())
  }
  ciphertext := gcm.Seal(nonce, nonce, []byte(data), nil)
  return ciphertext
}

func Decrypt(data []byte) string {
  // data := []byte(data_str)
  passphrase := os.Getenv("PASSPHRASE")
  key := []byte(createHash(passphrase))
  block, err := aes.NewCipher(key)
  if err != nil {
    panic(err.Error())
  }
  gcm, err := cipher.NewGCM(block)
  if err != nil {
    panic(err.Error())
  }
  nonceSize := gcm.NonceSize()
  nonce, ciphertext := data[:nonceSize], data[nonceSize:]
  plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
  if err != nil {
    panic(err.Error())
  }
  return string(plaintext)
}