AutoSleep API Documentation
----------------------------

All endpoints except `/drain/:app_id`, `/health`, `/healthz`, `/readyz`,
`/waking/:app_id` and the OAuth callback need HTTP basic auth (`API_USER` / `API_PASS`) and, if
`WHITELISTED_IPS` is set, a request from an allowed IP. The dashboard at `/`
uses this same API.

`:app_id` is your Heroku app name. Times are in seconds unless noted.
Errors look like `{"error": "message"}`.

| Field | Default | Meaning |
|---|---|---|
| `ideal_time` | 1800 | Seconds without traffic before the app sleeps (min 60). |
| `check_interval` | 600 | Upper bound between idle checks (min 30). Checks are also timed to the moment the app becomes idle. |
| `manual_mode` | false | Don't wake on traffic; wake only through the API or dashboard. |
| `night_mode` | false | Sleep only inside the night window (`NIGHT_MODE_START`, `NIGHT_MODE_HOURS`, `NIGHT_MODE_TZ`; default 15:30 UTC, i.e. 21:00 IST, for 12h). |
| `web_only` | false | Stop only `web` dynos; workers keep running. |
| `paused` | false | AutoSleep is disabled for the app. Pausing a sleeping app wakes it. |
| `wake_page` | false | Sets the app's `ERROR_PAGE_URL` to a friendly "waking up" page. Changing it restarts the app once. It won't overwrite an existing `ERROR_PAGE_URL`. |

An app object looks like:

```json
{
  "heroku_app_name": "my-staging",
  "status": "awake",              // awake | asleep | paused
  "current_status": true,
  "recent_activity_at": "2026-10-07T10:00:00Z",
  "next_sleep_at": "2026-10-07T10:30:00Z",
  "current_config": {"web": {"Quantity": "2", "Size": "Standard-1X"}},
  "ideal_time": 1800, "check_interval": 600,
  "manual_mode": false, "night_mode": false, "web_only": false, "paused": false, "wake_page": false,
  "last_error": "", "last_error_at": null,
  "account_id": 1, "account_email": "me@example.com"
}
```

API keys, OAuth tokens and drain tokens are never returned.

### Add an app

Use either an API key (`heroku authorizations:create -d autosleep`) or the
`account_id` of an account connected through `/register`.

```sh
curl -u $API_USER:$API_PASS -X POST https://<YOUR_DOMAIN>/app \
  -H 'Content-Type: application/json' \
  -d '{"heroku_app_name": "my-staging", "heroku_api_key": "<key>", "ideal_time": 900}'
```

Returns `409` if the app is already managed and `502` if Heroku rejects the
credentials or the log drain.

### List apps / get one app

```sh
curl -u $API_USER:$API_PASS https://<YOUR_DOMAIN>/app
curl -u $API_USER:$API_PASS https://<YOUR_DOMAIN>/app/:app_id
```

### Change settings

Send only the fields you want to change.

```sh
curl -u $API_USER:$API_PASS -X PATCH https://<YOUR_DOMAIN>/app/:app_id \
  -H 'Content-Type: application/json' -d '{"ideal_time": 600, "web_only": true}'
```

### Actions

```sh
curl -u $API_USER:$API_PASS -X POST https://<YOUR_DOMAIN>/app/:app_id/wake    # scale up now
curl -u $API_USER:$API_PASS -X POST https://<YOUR_DOMAIN>/app/:app_id/sleep   # scale down now
curl -u $API_USER:$API_PASS -X POST https://<YOUR_DOMAIN>/app/:app_id/pause   # disable AutoSleep (wakes it)
curl -u $API_USER:$API_PASS -X POST https://<YOUR_DOMAIN>/app/:app_id/resume  # re-enable, idle clock restarts
```

### Remove an app

This wakes the app if it is asleep and removes the log drain. If Heroku
fails, the request returns `502` and nothing is removed. Add `?force=true`
to remove the app from AutoSleep anyway.

```sh
curl -u $API_USER:$API_PASS -X DELETE https://<YOUR_DOMAIN>/app/:app_id
```

### History and savings

```sh
curl -u $API_USER:$API_PASS 'https://<YOUR_DOMAIN>/app/:app_id/history?limit=100'
curl -u $API_USER:$API_PASS 'https://<YOUR_DOMAIN>/app/:app_id/savings?days=30'
curl -u $API_USER:$API_PASS 'https://<YOUR_DOMAIN>/savings?days=30'    # all apps
```

A savings report contains `saved_usd`, `slept_dyno_hours`,
`sleeping_percent`, `projected_monthly_usd` and a per-dyno `breakdown`. Prices
are Heroku list prices; override them with `DYNO_HOURLY_PRICES`.

### Heroku accounts

```sh
open https://<YOUR_DOMAIN>/register                                      # connect via OAuth (browser)
curl -u $API_USER:$API_PASS https://<YOUR_DOMAIN>/accounts               # connected accounts
curl -u $API_USER:$API_PASS https://<YOUR_DOMAIN>/accounts/:id/apps      # their apps, with "managed" flags
```

### Health

`GET /health` (alias `GET /readyz`) returns `200` with
`{"database":"ok","redis":"ok"}`, or `503` if either is unreachable.

`GET /healthz` is a liveness probe: `200` whenever the process is running,
without checking dependencies.

### Metrics

`GET /metrics` (basic auth) serves Prometheus metrics:
`autosleep_scale_events_total{action,reason}`, `autosleep_app_errors_total`,
`autosleep_drain_rejected_total{reason}`, `autosleep_worker_jobs_total{name,outcome}`
and `autosleep_http_requests_total{route,status}`, plus Go runtime metrics.

The OAuth callback and `/waking/:app_id` are rate limited per client IP
(5 requests/second, bursts of 20); excess requests get `429`.

### Log drain (called by Heroku)

`POST /drain/:app_id` receives `application/logplex-1` batches. The
`Logplex-Drain-Token` header must match the drain's token. Only Heroku
router lines count as traffic.
