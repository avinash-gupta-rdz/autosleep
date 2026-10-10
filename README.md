AutoSleep
=========

**Sleep your Heroku dynos while nobody is using them.**

AutoSleep watches your staging, review and demo apps. When an app gets no
HTTP traffic for a while (30 minutes by default) it scales the dynos to zero.
The next request wakes it up again with exactly the same dyno count and
sizes, usually in 10–30 seconds.

It is an open-source, self-hosted alternative to the paid
[AutoIdle](https://elements.heroku.com/addons/autoidle) add-on.

[![Deploy to Heroku](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/avinash-gupta-rdz/autosleep)

> Use AutoSleep for non-production apps only. The first visitor after a
> sleep waits while the app boots.

## Features

| | |
|---|---|
| **Idle detection** | Uses the app's Heroku router logs (via a log drain) to see real HTTP requests. ACME `/.well-known/` checks are ignored. |
| **Wake on request** | The first request after a sleep scales the app back up to its saved formation. |
| **Dashboard** | See which apps are awake, asleep or paused and when each will sleep next, and wake, sleep or pause an app with one click. |
| **Savings report** | Dollars saved, dyno-hours slept, % of time asleep and a monthly projection, priced from Heroku's list prices. |
| **One-click enrol** | Connect your Heroku account and enable AutoSleep for any of your apps from the dashboard. |
| **Review apps** | Set `AUTO_ENROLL=myapp-pr-*` and new review apps are enrolled automatically. Deleted apps are cleaned up. |
| **Waking page** | Optional friendly "this app is waking up" page instead of Heroku's error page. |
| **Per-app settings** | Idle timeout, web-only mode (workers keep running), night mode (sleep only at night), manual wake, pause. |
| **Observability** | `/healthz` and `/readyz` probes, and Prometheus metrics at `/metrics` (scale events, errors, rejected drain posts, background jobs). |
| **Secure by default** | Tokens encrypted at rest (AES-256-GCM, Argon2id-derived key), authenticated log drains, one-time OAuth state, IP allowlist, cross-site request blocking and rate-limited public pages. See [SECURITY.md](SECURITY.md). |
| **Reliable** | Saved formations are never lost on API errors, and state changes use compare-and-set updates so concurrent requests can't double-scale an app. If an app is marked awake but its dynos are stopped, the next request restores them. A reconciler re-checks every app every 5 minutes. |

## Deploy on Heroku (5 minutes)

### Option A: Deploy button

1. Click **Deploy to Heroku** above. This creates an app with Heroku
   Postgres, Heroku Key-Value Store (Redis) and a Basic dyno, and generates
   `API_PASS` and `PASSPHRASE` for you.
2. Open the app. Log in as `admin` with the password from
   `heroku config:get API_PASS -a <your-autosleep-app>`.
3. Add your first app (see [Adding apps](#adding-apps)).

### Option B: Heroku CLI

```sh
git clone https://github.com/avinash-gupta-rdz/autosleep && cd autosleep
heroku create my-autosleep
heroku addons:create heroku-postgresql:essential-0
heroku addons:create heroku-redis:mini
heroku config:set API_USER=admin API_PASS=$(openssl rand -hex 16) PASSPHRASE=$(openssl rand -hex 32)
git push heroku master
heroku open
```

**Run AutoSleep itself on a Basic (or larger) dyno.** An Eco dyno sleeps,
and a sleeping AutoSleep can't wake your other apps.

Running cost is about $15/month (Basic dyno $7, Postgres Essential-0 $5,
Key-Value Store Mini $3). Two sleeping Standard-1X apps usually save more
than that.

## Adding apps

There are two ways to give AutoSleep access to an app.

**Connect your Heroku account (recommended).** This needs a one-time
OAuth client:

1. Go to <https://dashboard.heroku.com/account/applications> and click
   **Register new API client**.
2. Set the callback URL to `https://<your-autosleep-app>.herokuapp.com/auth/heroku/callback`.
3. `heroku config:set HEROKU_OAUTH_ID=<id> HEROKU_OAUTH_SECRET=<secret>`
4. In the dashboard click **Connect Heroku account**, then **Add app** and
   pick apps from the list.

**Or use an API key.** Create one with `heroku authorizations:create -d autosleep`,
then in the dashboard choose **Add app → Use an API key**. You can also call
the [API](api_doc.md) directly:

```sh
curl -u admin:$API_PASS https://<your-autosleep-app>.herokuapp.com/app \
  -H 'Content-Type: application/json' \
  -d '{"heroku_app_name": "my-staging-app", "heroku_api_key": "<key>"}'
```

When you add an app, AutoSleep adds a log drain to it
(`heroku drains -a my-staging-app` shows it). Removing the app from
AutoSleep wakes it if needed and removes the drain.

### Review apps

Connect your Heroku account, then set a pattern for the apps to manage:

```sh
heroku config:set AUTO_ENROLL='myapp-pr-*' AUTO_ENROLL_IDLE_MINUTES=15
```

Every 5 minutes AutoSleep enrols new apps that match. When a review app is
deleted, AutoSleep forgets it. Separate several patterns with commas.

## Configuration

| Config var | Required | Description |
|---|---|---|
| `API_USER`, `API_PASS` | yes | Dashboard/API login (HTTP basic auth). |
| `PASSPHRASE` | yes | Encrypts stored Heroku tokens. **Never change it** after setup. |
| `DATABASE_URL` | yes | Postgres (`postgres://…`) or MySQL (`mysql://…` or a go-sql-driver DSN). `JAWSDB_URL` and `CLEARDB_DATABASE_URL` are also detected. |
| `REDIS_URL` | yes | Also accepts `REDISCLOUD_URL` or `REDIS_TLS_URL`. TLS (`rediss://`) is supported. |
| `HEROKU_OAUTH_ID`, `HEROKU_OAUTH_SECRET` | no | Enables **Connect Heroku account**. |
| `SELF_HOST` | no | Public URL of AutoSleep. Detected from requests if unset. Set it if you use `AUTO_ENROLL` and want enrolment to work right after a restart, before anyone opens the dashboard. |
| `WHITELISTED_IPS` | no | Comma-separated CIDRs or IPs allowed to use the dashboard/API. Empty means any IP. |
| `AUTO_ENROLL` | no | App name globs to enrol automatically, e.g. `myapp-pr-*,demo-*`. |
| `AUTO_ENROLL_IDLE_MINUTES` | no | Idle timeout for auto-enrolled apps (default 30). |
| `NIGHT_MODE_TZ` | no | Time zone for night mode, e.g. `Asia/Kolkata` (default `UTC`). |
| `NIGHT_MODE_START` | no | Night window start, `HH:MM` in that zone (default `15:30`). |
| `NIGHT_MODE_HOURS` | no | Night window length (default `12`). |
| `DYNO_HOURLY_PRICES` | no | Override prices for the savings report, e.g. `private-s=0.175,standard-1x=0.035`. |
| `WORKER_CONCURRENCY` | no | Parallel background jobs (default 10). |

## How it works

1. AutoSleep adds an HTTPS log drain to each managed app. Heroku sends
   the app's logs to `/drain/<app>`, signed with a per-drain token that
   AutoSleep checks.
2. Router log lines mean real HTTP requests, and they reset the idle clock.
   While the app sleeps, requests still produce router logs (`H14 No web
   processes running`), and those wake it.
3. A background job checks each awake app at the moment it would become
   idle. If there has been no traffic, it saves the formation and scales
   everything (or only `web`, in web-only mode) to zero.
4. Every sleep and wake is recorded. The savings report is built from that
   history.

```
visitor ──► Heroku router ──► your app (0 dynos)
                 │
                 └─ router log ──► AutoSleep /drain/<app> ──► scale back up
```

## Local development

```sh
docker compose up                        # AutoSleep + Postgres + Redis on http://localhost:8080
# or, with your own database and Redis:
cp sample_env_vars .env && source .env   # needs MySQL or Postgres, and Redis
go run .
go test ./...                            # tests use in-memory SQLite and Redis, no services needed
```

The `Dockerfile` builds a small static image you can run anywhere, not just
on Heroku. See [CONTRIBUTING.md](CONTRIBUTING.md) to get involved.

## API

See [api_doc.md](api_doc.md).

## License

[Apache 2.0](LICENSE).
