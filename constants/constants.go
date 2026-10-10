package constants

import (
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/heroku"
)

// Defaults for new apps, in seconds.
const CheckInterval = 600
const IdealTime = 1800

var Passphrase = os.Getenv("PASSPHRASE")

// SelfHost is the public URL of this deployment. Optional: when empty it is
// derived from incoming requests.
var SelfHost = os.Getenv("SELF_HOST")

// OauthConfig's RedirectURL is filled in per request (see controllers).
var OauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("HEROKU_OAUTH_ID"),
	ClientSecret: os.Getenv("HEROKU_OAUTH_SECRET"),
	Endpoint:     heroku.Endpoint,
	Scopes:       []string{"identity", "read", "write"}, // See https://devcenter.heroku.com/articles/oauth#scopes
}

var AllowedIps = os.Getenv("WHITELISTED_IPS")
