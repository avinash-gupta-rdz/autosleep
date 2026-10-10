package models

import (
	"autosleep/constants"
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type Account struct {
	gorm.Model
	Email   string `gorm:"type:varchar(255);index:idx_email,unique" json:"email"`
	Name    string `json:"name"`
	Country string `json:"country"`

	AccessToken  []byte    `json:"-"`
	RefreshToken []byte    `json:"-"`
	ExpiredAt    time.Time `json:"expired_at"`
}

// TokenExpiry returns when a freshly issued token should be considered
// expired. Heroku OAuth tokens live 8 hours; we refresh 5 minutes early.
func TokenExpiry(token *oauth2.Token) time.Time {
	if !token.Expiry.IsZero() {
		return token.Expiry.UTC().Add(-5 * time.Minute)
	}
	return time.Now().UTC().Add(7*time.Hour + 55*time.Minute)
}

// CurrentAccessToken returns a valid access token, refreshing it first if it
// has expired.
func (act *Account) CurrentAccessToken() (string, error) {
	if time.Now().UTC().Before(act.ExpiredAt) {
		return Decrypt(act.AccessToken)
	}
	return act.refreshToken()
}

func (act *Account) refreshToken() (string, error) {
	access, err := Decrypt(act.AccessToken)
	if err != nil {
		return "", err
	}
	refresh, err := Decrypt(act.RefreshToken)
	if err != nil {
		return "", err
	}
	token := &oauth2.Token{AccessToken: access, RefreshToken: refresh, Expiry: act.ExpiredAt, TokenType: "Bearer"}
	token, err = constants.OauthConfig.TokenSource(context.Background(), token).Token()
	if err != nil {
		return "", fmt.Errorf("refreshing Heroku token for %s: %w", act.Email, err)
	}

	act.AccessToken = Encrypt(token.AccessToken)
	if token.RefreshToken != "" {
		act.RefreshToken = Encrypt(token.RefreshToken)
	}
	act.ExpiredAt = TokenExpiry(token)
	if DB != nil {
		err = DB.Model(&Account{}).Where("id = ?", act.ID).Updates(map[string]interface{}{
			"access_token": act.AccessToken, "refresh_token": act.RefreshToken, "expired_at": act.ExpiredAt,
		}).Error
		if err != nil {
			return "", err
		}
	}
	return token.AccessToken, nil
}
