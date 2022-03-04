package constants

import (
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/heroku"
	"os"
)

const CheckInterval = 600
const IdealTime = 1800
const NightModeHour = 12

var NightModeStart = map[string]int{"hour": 15, "minute": 30}

var Passphrase = os.Getenv("PASSPHRASE")
var SelfHost = os.Getenv("SELF_HOST")

var OauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("HEROKU_OAUTH_ID"),
	ClientSecret: os.Getenv("HEROKU_OAUTH_SECRET"),
	Endpoint:     heroku.Endpoint,
	Scopes:       []string{"identity", "read", "write"},          // See https://devcenter.heroku.com/articles/oauth#scopes
	RedirectURL:  "http://" + SelfHost + "/auth/heroku/callback", // See https://devcenter.heroku.com/articles/dyno-metadata
}

var StateToken = os.Getenv("HEROKU_APP_NAME")
