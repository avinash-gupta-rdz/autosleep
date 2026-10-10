package controllers

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"context"
	"net/http"
	"sort"

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
