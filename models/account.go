package models

import (
	"autosleep/constants"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"github.com/jinzhu/gorm"
	"golang.org/x/oauth2"
	"io"
	"time"
)

type Account struct {
	gorm.Model
	Email   string `gorm:"type:varchar(255);index:idx_email,unique",json:"email"`
	Name    string `json:"name"`
	Country string `json:"country"`

	AccessToken  []byte    `json:"access_token"`
	RefreshToken []byte    `json:"refresh_token"`
	ExpiredAt    time.Time `json:"expired_at"`
}

func (act *Account) CurrentAccessToken() string {
	current := time.Now().UTC()
	diff := act.ExpiredAt.Sub(current)
	if diff.Seconds() < 0 {
		return act.refreshToken()
	} else {
		return Decrypt(act.AccessToken)
	}

}

func (act *Account) refreshToken() string {
	token := new(oauth2.Token)
	token.AccessToken = Decrypt(act.AccessToken)
	token.RefreshToken = Decrypt(act.RefreshToken)
	token.Expiry = act.ExpiredAt
	token.TokenType = "Bearer"
	token, err := constants.OauthConfig.TokenSource(context.Background(), token).Token()
	if err != nil {
		panic(err.Error())
	}

	expired_at := time.Now().UTC().Add(time.Hour*time.Duration(7) +
		time.Minute*time.Duration(55))
	DB.Model(&act).Updates(map[string]interface{}{"access_token": Encrypt(token.AccessToken), "refresh_token": Encrypt(token.RefreshToken), "expired_at": expired_at})
	return token.AccessToken
}

func Encrypt(data string) []byte {
	gcm := createHashGcm()
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err.Error())
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(data), nil)
	return ciphertext
}

func Decrypt(data []byte) string {
	gcm := createHashGcm()
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		panic(err.Error())
	}
	return string(plaintext)
}

func createHashGcm() cipher.AEAD {
	hasher := md5.New()
	hasher.Write([]byte(constants.Passphrase))
	hash := hex.EncodeToString(hasher.Sum(nil))

	block, _ := aes.NewCipher([]byte(hash))
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}
	return gcm
}

//  func createHash(key string) string {
//   hasher := md5.New()
//   hasher.Write([]byte(key))
//   return hex.EncodeToString(hasher.Sum(nil))
// }
