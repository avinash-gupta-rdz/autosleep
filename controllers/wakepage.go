package controllers

import (
	"autosleep/herokuclient"
	"autosleep/models"
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// errorPageVar is Heroku's config var for the page shown on router errors
// such as H14 "No web processes running" — i.e. while an app sleeps.
const errorPageVar = "ERROR_PAGE_URL"

func wakePageURL(base, app string) string { return base + "/waking/" + app }

// setWakePage points the app's ERROR_PAGE_URL at our waking page, or
// removes it. It refuses to overwrite an error page the user set up
// themselves. Changing a config var restarts the app's dynos.
func setWakePage(ctx context.Context, app *models.Application, base string, enable bool) error {
	token, err := app.GetToken()
	if err != nil {
		return err
	}
	hc := herokuclient.New(token)
	current, err := hc.GetConfigVar(ctx, app.HerokuAppName, errorPageVar)
	if err != nil {
		return err
	}
	ours := strings.HasSuffix(current, "/waking/"+app.HerokuAppName)
	if enable {
		if current != "" && !ours {
			return fmt.Errorf("%s already has %s=%s; remove it first to use the AutoSleep waking page", app.HerokuAppName, errorPageVar, current)
		}
		url := wakePageURL(base, app.HerokuAppName)
		if current == url {
			return nil
		}
		return hc.SetConfigVar(ctx, app.HerokuAppName, errorPageVar, &url)
	}
	if !ours {
		return nil // never remove a page we didn't set
	}
	return hc.SetConfigVar(ctx, app.HerokuAppName, errorPageVar, nil)
}

var wakingTmpl = template.Must(template.New("waking").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Waking up {{.}}</title>
<style>
:root{color-scheme:light dark;--bg:#f7f7f8;--fg:#1d1d1f;--muted:#6e6e73;--accent:#6c4cf1}
@media (prefers-color-scheme:dark){:root{--bg:#141416;--fg:#f2f2f4;--muted:#a1a1a6;--accent:#9d86ff}}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,-apple-system,Segoe UI,sans-serif;padding:16px;box-sizing:border-box;text-align:center}
.card{max-width:420px}.moon{font-size:48px}.bar{height:4px;background:color-mix(in srgb,var(--accent) 20%,transparent);border-radius:2px;overflow:hidden;margin:24px 0}
.bar i{display:block;height:100%;width:0;background:var(--accent);animation:fill 25s linear forwards}
@keyframes fill{to{width:100%}}p{color:var(--muted)}
a.btn{display:inline-block;padding:10px 18px;border-radius:8px;background:var(--accent);color:#fff;text-decoration:none;font-weight:600}
</style></head><body><div class="card">
<div class="moon" aria-hidden="true">☾</div>
<h1>{{.}} is waking up</h1>
<p>This app sleeps when nobody is using it, to save energy and money. Your visit just woke it up. It's usually ready in 10–30 seconds.</p>
<div class="bar"><i></i></div>
<a class="btn" id="go" target="_top" href="/">Try again</a>
</div>
<script>
// Heroku shows this page in an iframe; send the visitor back to the app.
try { if (document.referrer) document.getElementById('go').href = document.referrer; } catch (e) {}
</script></body></html>`))

// WakingPage is the public page shown (via ERROR_PAGE_URL) while an app
// sleeps. It reveals nothing beyond the app name already in the URL.
func WakingPage(c *gin.Context) {
	var buf bytes.Buffer
	if err := wakingTmpl.Execute(&buf, c.Param("app_id")); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "text/html; charset=utf-8", buf.Bytes())
}
