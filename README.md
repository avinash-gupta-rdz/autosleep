AutoSleep
=========

**Sleep your Heroku dynos while nobody is using them. Free and open source.**

AutoSleep watches your staging, review and demo apps. When an app gets no
HTTP traffic for a while (30 minutes by default) it scales the dynos to zero.
The next request wakes it up again with exactly the same dyno count and
sizes, usually in 10–30 seconds.

One AutoSleep deployment looks after all your apps, for about $15/month of
your own Heroku resources. It is a self-hosted alternative to the paid
[AutoIdle](https://elements.heroku.com/addons/autoidle) add-on, which costs
$4–150 per app per month ([comparison](docs/comparison.md)).

[![Deploy to Heroku](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/avinash-gupta-rdz/autosleep)

> Use AutoSleep for non-production apps only. The first visitor after a
> sleep waits while the app boots.

## Try it in one minute

No Heroku account, database or Redis needed: demo mode runs everything in
memory against a simulated Heroku account with a few apps, a month of
savings history and some live traffic.

```sh
git clone https://github.com/avinash-gupta-rdz/autosleep && cd autosleep
DEMO_MODE=true go run .           # or: docker build -t autosleep . && docker run -p 8080:8080 -e DEMO_MODE=true autosleep
```

Open <http://localhost:8080> and log in as `demo` / `demo`. Nothing real is
scaled.

## Set up for real (about 5 minutes)

1. **Create a Heroku API key** for AutoSleep:
   ```sh
   heroku authorizations:create -d autosleep
   ```
   Copy the `Token` it prints.
2. **Click Deploy to Heroku** above. Paste the token into `HEROKU_API_KEY`
   (or skip it and paste it in the dashboard later). The button creates the
   app with Postgres, Redis and a Basic dyno, and generates the dashboard
   password and encryption passphrase for you.
3. **Open the app** and log in as `admin` with the password from
   `heroku config:get API_PASS -a <your-autosleep-app>`.
4. **Tick the apps** AutoSleep should manage and click **Enable**. The
   "Get started" checklist on the dashboard shows when logs start arriving
   and when the first app goes to sleep.

Run AutoSleep itself on a Basic (or larger) dyno: an Eco dyno sleeps, and a
sleeping AutoSleep can't wake your other apps.

Prefer the CLI?

```sh
git clone https://github.com/avinash-gupta-rdz/autosleep && cd autosleep
heroku create my-autosleep
heroku addons:create heroku-postgresql:essential-0
heroku addons:create heroku-redis:mini
heroku config:set API_USER=admin API_PASS=$(openssl rand -hex 16) PASSPHRASE=$(openssl rand -hex 32) \
  HEROKU_API_KEY=$(heroku authorizations:create -d autosleep -S)
git push heroku master
heroku open
```

## Features

| | |
|---|---|
| **Wake on request** | Real HTTP requests (seen through a log drain AutoSleep adds to each app) keep an app awake; the first request after a sleep scales it back to its saved formation. ACME `/.well-known/` checks are ignored. |
| **Waking page** | Visitors see a friendly "waking up" page instead of Heroku's error page, on your app's own domain. On by default. |
| **Office hours** | Keep an app awake during working hours (e.g. Mon–Fri 09:00–18:00 in your time zone) and wake it when they start. Outside them it sleeps when idle and still wakes on the next visit. |
| **Dashboard** | See which apps are awake, asleep or paused and when each will sleep next; wake, sleep or pause with one click; enable many apps at once. |
| **Savings report** | Dollars saved, dyno-hours slept, % of time asleep and a monthly projection, priced from Heroku's list prices. |
| **Self-checks** | **Check** on any app verifies access, the log drain and that logs are arriving, explains what's wrong in plain words, and re-installs the drain with one click. |
| **Alerts** | Failed wakes, recoveries and a Monday savings digest to Slack, Discord or any webhook. |
| **Review apps** | `AUTO_ENROLL=myapp-pr-*` enrols new review apps automatically; deleted apps are cleaned up. |
| **Team logins** | Extra admin or read-only logins for colleagues (`USERS`). |
| **Per-app settings** | Idle timeout, web-only mode (workers keep running), night mode, manual wake, pause. |
| **Reliable** | Saved formations are never lost on API errors; state changes use compare-and-set updates so concurrent requests can't double-scale an app; stopped dynos on an "awake" app are restored on the next request; a reconciler re-checks every app every 5 minutes. |
| **Observability** | `/healthz` and `/readyz` probes and Prometheus metrics at `/metrics`. |
| **Secure by default** | Tokens encrypted at rest (AES-256-GCM, Argon2id-derived key), authenticated log drains, IP allowlist, cross-site request blocking. See [SECURITY.md](SECURITY.md). |

## Adding apps

From the dashboard: **Add app** → tick apps → **Enable**. If no Heroku
account is connected yet, the dialog asks for an API key first. Each app
gets a log drain (`heroku drains -a my-app` shows it); removing the app from
AutoSleep wakes it if needed and removes the drain.

Other ways:

- **Heroku login (OAuth)** instead of an API key: register an API client at
  <https://dashboard.heroku.com/account/applications> with callback URL
  `https://<your-autosleep-app>.herokuapp.com/auth/heroku/callback`, then
  `heroku config:set HEROKU_OAUTH_ID=<id> HEROKU_OAUTH_SECRET=<secret>`.
- **One app with its own key**: **Add app → Use an API key**.
- **The [API](api_doc.md)**:
  ```sh
  curl -u admin:$API_PASS https://<your-autosleep-app>.herokuapp.com/app \
    -H 'Content-Type: application/json' \
    -d '{"heroku_app_name": "my-staging-app", "heroku_api_key": "<key>"}'
  ```

### Review apps

```sh
heroku config:set AUTO_ENROLL='myapp-pr-*' AUTO_ENROLL_IDLE_MINUTES=15 SELF_HOST=https://<your-autosleep-app>.herokuapp.com
```

Every 5 minutes AutoSleep enrols new apps that match (separate several
patterns with commas). When a review app is deleted, AutoSleep forgets it.

## Configuration

| Config var | Required | Description |
|---|---|---|
| `API_USER`, `API_PASS` | yes | Admin login for the dashboard and API (HTTP basic auth). |
| `PASSPHRASE` | yes | Encrypts stored Heroku keys. **Never change it** after setup. |
| `DATABASE_URL` | yes | Postgres (`postgres://…`) or MySQL (`mysql://…` or a go-sql-driver DSN). `JAWSDB_URL` and `CLEARDB_DATABASE_URL` are also detected. |
| `REDIS_URL` | yes | Also accepts `REDISCLOUD_URL` or `REDIS_TLS_URL`. TLS (`rediss://`) is supported. |
| `HEROKU_API_KEY` | no | Connects this Heroku account at start-up so its apps can be enabled right away. |
| `HEROKU_OAUTH_ID`, `HEROKU_OAUTH_SECRET` | no | Enables connecting accounts with a Heroku login. |
| `USERS` | no | More logins: `name:password:role,…` with role `admin` or `viewer` (read-only). |
| `ALERT_WEBHOOK_URL` | no | Slack/Discord/webhook URL for alerts and the weekly digest. |
| `WEEKLY_DIGEST` | no | `false` turns off the Monday savings digest. |
| `SELF_HOST` | no | Public URL of AutoSleep. Detected from requests if unset; set it when using `AUTO_ENROLL`. |
| `WHITELISTED_IPS` | no | Comma-separated CIDRs or IPs allowed to use the dashboard/API. Empty means any IP. |
| `AUTO_ENROLL` | no | App name globs to enrol automatically, e.g. `myapp-pr-*,demo-*`. |
| `AUTO_ENROLL_IDLE_MINUTES` | no | Idle timeout for auto-enrolled apps (default 30). |
| `WAKE_PAGE_DEFAULT` | no | `false` stops new apps getting the waking page (setting it restarts the app once). |
| `NIGHT_MODE_TZ`, `NIGHT_MODE_START`, `NIGHT_MODE_HOURS` | no | The window night-mode apps may sleep in (default 15:30 UTC for 12 hours). |
| `DYNO_HOURLY_PRICES` | no | Override prices for the savings report, e.g. `private-s=0.175,standard-1x=0.035`. |
| `WORKER_CONCURRENCY` | no | Parallel background jobs (default 10). |
| `UPDATE_CHECK` | no | `false` stops the daily check for new releases on GitHub. |
| `DEMO_MODE` | no | `true` runs the in-memory demo described above. |

## How it works

1. AutoSleep adds an HTTPS log drain to each managed app. Heroku sends the
   app's logs to `/drain/<app>`, signed with a per-drain token that
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

Questions about WebSockets, Private Spaces, the new Fir generation or Eco
dynos are answered in the [FAQ](docs/faq.md).

## Upgrading

The dashboard tells you when a new release is out. See
[UPGRADING.md](UPGRADING.md); upgrades keep all your data.

## Local development

```sh
DEMO_MODE=true go run .                  # no services needed
docker compose up                        # AutoSleep + Postgres + Redis on http://localhost:8080
go test ./...                            # in-memory SQLite, Redis and Heroku; no services needed
```

See [CONTRIBUTING.md](CONTRIBUTING.md) to get involved.

## API

See [api_doc.md](api_doc.md).

## License

[Apache 2.0](LICENSE).
