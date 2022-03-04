package controllers

import (
	"autosleep/constants"
	"autosleep/models"
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm/clause"
	"net/http"
	"time"
)

func HandleAuth(c *gin.Context) {
	url := constants.OauthConfig.AuthCodeURL(constants.StateToken)
	c.Redirect(http.StatusFound, url)
}

func HandleAuthCallback(c *gin.Context) {
	fmt.Println("params: ", c.Query("state"))
	fmt.Println("const ", constants.StateToken)
	if v := c.Query("state"); v != constants.StateToken {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid State token"})
		return
	}
	ctx := context.Background()
	token, err := constants.OauthConfig.Exchange(ctx, c.Query("code"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	account := saveAccountInfo(token)
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprint("Welcome ", account.Name),
		"account_id": account.ID,
		"email":      account.Email})
}

func saveAccountInfo(token *oauth2.Token) models.Account {
	expired_at := time.Now().UTC().Add(
		time.Hour*time.Duration(7) +
			time.Minute*time.Duration(55))
	account_info := GetAccountInformation(token.AccessToken)

	account := models.Account{
		Email: account_info.Email,
		Name:  *account_info.Name,
		// Country:      *account_info.CountryOfResidence,
		AccessToken:  models.Encrypt(token.AccessToken),
		RefreshToken: models.Encrypt(token.RefreshToken),
		ExpiredAt:    expired_at,
	}

	models.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{"access_token", "refresh_token", "expired_at", "name"}),
	}).Create(&account) // upsert record on email
	return account
}
