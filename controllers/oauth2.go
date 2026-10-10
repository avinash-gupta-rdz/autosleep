package controllers

import (
	"autosleep/constants"
	"autosleep/herokuclient"
	"autosleep/models"
	"crypto/rand"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm/clause"
)

const stateCookie = "autosleep_oauth_state"

func oauthConfig(c *gin.Context) *oauth2.Config {
	cfg := *constants.OauthConfig
	cfg.RedirectURL = BaseURL(c) + "/auth/heroku/callback"
	return &cfg
}

// HandleAuth starts the Heroku OAuth flow with a one-time state value kept
// in a short-lived cookie (CSRF protection).
func HandleAuth(c *gin.Context) {
	if constants.OauthConfig.ClientID == "" {
		jsonError(c, http.StatusServiceUnavailable, "Heroku OAuth is not configured: set HEROKU_OAUTH_ID and HEROKU_OAUTH_SECRET (see README)")
		return
	}
	state := rand.Text()
	secure := strings.HasPrefix(BaseURL(c), "https://")
	c.SetCookie(stateCookie, state, 600, "/auth/heroku", "", secure, true)
	c.Redirect(http.StatusFound, oauthConfig(c).AuthCodeURL(state))
}

func HandleAuthCallback(c *gin.Context) {
	want, err := c.Cookie(stateCookie)
	got := c.Query("state")
	if err != nil || want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		jsonError(c, http.StatusBadRequest, "Invalid or expired OAuth state; start again from /register")
		return
	}
	c.SetCookie(stateCookie, "", -1, "/auth/heroku", "", false, true)

	token, err := oauthConfig(c).Exchange(c.Request.Context(), c.Query("code"))
	if err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	account, err := saveAccountInfo(c, token)
	if err != nil {
		jsonError(c, http.StatusBadGateway, err.Error())
		return
	}
	if strings.Contains(c.GetHeader("Accept"), "text/html") {
		c.Redirect(http.StatusFound, "/?connected="+url.QueryEscape(account.Email))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Welcome " + account.Name, "account_id": account.ID, "email": account.Email})
}

func saveAccountInfo(c *gin.Context, token *oauth2.Token) (models.Account, error) {
	email, name, err := herokuclient.New(token.AccessToken).AccountInfo(c.Request.Context())
	if err != nil {
		return models.Account{}, err
	}
	return upsertAccount(models.Account{
		Email:        email,
		Name:         name,
		AuthType:     models.AuthOAuth,
		AccessToken:  models.Encrypt(token.AccessToken),
		RefreshToken: models.Encrypt(token.RefreshToken),
		ExpiredAt:    models.TokenExpiry(token),
	})
}

// upsertAccount saves an account keyed on email, replacing the credentials
// of an existing one (so reconnecting fixes a revoked token).
func upsertAccount(account models.Account) (models.Account, error) {
	err := models.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{"access_token", "refresh_token", "expired_at", "name", "auth_type"}),
	}).Create(&account).Error
	if err != nil {
		return account, err
	}
	// On conflict the returned ID may be empty; read it back.
	err = models.DB.Where("email = ?", account.Email).First(&account).Error
	return account, err
}
