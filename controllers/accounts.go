package controllers

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

func ListAccounts(c *gin.Context) {
	var accounts []models.Account
	models.DB.Order("email").Find(&accounts)
	c.JSON(http.StatusOK, gin.H{"list": accounts})
}

// HerokuAppView is an app in a connected Heroku account and whether
// AutoSleep already manages it.
type HerokuAppView struct {
	Name    string `json:"name"`
	WebURL  string `json:"web_url"`
	Managed bool   `json:"managed"`
}

// AccountApps lists the Heroku apps a connected account can see, so the
// dashboard can enrol them with one click.
func AccountApps(c *gin.Context) {
	var act models.Account
	if err := models.DB.First(&act, c.Param("account_id")).Error; err != nil {
		jsonError(c, http.StatusNotFound, "Account not found")
		return
	}
	token, err := act.CurrentAccessToken()
	if err != nil {
		jsonError(c, http.StatusBadGateway, err.Error()+" (reconnect the account via /register)")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	apps, err := herokuclient.New(token).ListApps(ctx)
	if err != nil {
		jsonError(c, http.StatusBadGateway, err.Error())
		return
	}
	var managed []string
	models.DB.Model(&models.Application{}).Pluck("heroku_app_name", &managed)
	isManaged := map[string]bool{}
	for _, m := range managed {
		isManaged[m] = true
	}
	out := make([]HerokuAppView, 0, len(apps))
	for _, a := range apps {
		out = append(out, HerokuAppView{Name: a.Name, WebURL: a.WebURL, Managed: isManaged[a.Name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	c.JSON(http.StatusOK, gin.H{"list": out})
}

// ErrBadAPIKey means Heroku did not accept the key.
var ErrBadAPIKey = errors.New("Heroku rejected this API key; create one with `heroku authorizations:create -d autosleep`")

// ConnectAPIKey verifies a Heroku API key and saves it as a connected
// account, so its apps can be enrolled from the dashboard. Unlike OAuth this
// needs no OAuth client: `heroku authorizations:create -d autosleep` prints
// a key that never expires.
func ConnectAPIKey(ctx context.Context, key string) (models.Account, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return models.Account{}, fmt.Errorf("%w: api_key is required", ErrBadAPIKey)
	}
	email, name, err := herokuclient.New(key).AccountInfo(ctx)
	if err != nil {
		return models.Account{}, fmt.Errorf("%w (%v)", ErrBadAPIKey, err)
	}
	return upsertAccount(models.Account{
		Email:        email,
		Name:         name,
		AuthType:     models.AuthAPIKey,
		AccessToken:  models.Encrypt(key),
		RefreshToken: nil,
	})
}

// CreateAccount connects a Heroku account with an API key.
func CreateAccount(c *gin.Context) {
	var in struct {
		APIKey string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), herokuTimeout)
	defer cancel()
	act, err := ConnectAPIKey(ctx, in.APIKey)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrBadAPIKey) {
			status = http.StatusBadRequest
		}
		jsonError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": act})
}

// DeleteAccount disconnects a Heroku account. Apps still using it must be
// removed (or given their own API key) first, or they could no longer be
// woken.
func DeleteAccount(c *gin.Context) {
	var act models.Account
	if err := models.DB.First(&act, c.Param("account_id")).Error; err != nil {
		jsonError(c, http.StatusNotFound, "Account not found")
		return
	}
	var n int64
	models.DB.Model(&models.Application{}).
		Where("account_id = ? AND (heroku_api_key IS NULL OR length(heroku_api_key) = 0)", act.ID).Count(&n)
	if n > 0 {
		jsonError(c, http.StatusConflict, fmt.Sprintf("%d app(s) still use this account; remove them first", n))
		return
	}
	if err := models.DB.Unscoped().Delete(&act).Error; err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "Disconnected " + act.Email})
}
