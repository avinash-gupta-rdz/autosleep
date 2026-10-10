# Changelog

All notable changes to AutoSleep are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project aims to adhere to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) from `v0.1.0` onward.

## [Unreleased]

### Added
- Demo mode (`DEMO_MODE=true`): try the dashboard with an in-memory database and Redis and a simulated Heroku account, no setup needed.
- First-run "Get started" checklist on the dashboard (`GET /setup`).
- Connect a Heroku account by pasting an API key (`POST /accounts`) or with `HEROKU_API_KEY` at deploy time; no OAuth client required. Accounts can be disconnected.
- Enable many apps at once from the add dialog.
- Per-app **Check** (`GET /app/:id/diagnose`): Heroku access, log drain, logs arriving (`last_drain_at`), saved formation and last error, with a one-click drain re-install (`POST /app/:id/repair-drain`).
- The waking page is on by default for new and auto-enrolled apps (`WAKE_PAGE_DEFAULT=false` opts out).
- Per-app office hours (`awake_days`, `awake_from`, `awake_to`, `awake_tz`): kept awake and woken at the start, wake-on-request outside.
- Alerts to Slack, Discord or any webhook (`ALERT_WEBHOOK_URL`) for failed sleeps/wakes, recoveries and deleted apps, plus a Monday savings digest (`WEEKLY_DIGEST=false` to disable).
- Team logins with admin or read-only viewer roles (`USERS`), `GET /me`.
- Version shown in the dashboard with a daily check for new releases (`UPDATE_CHECK=false` to disable), and [UPGRADING.md](UPGRADING.md).
- Docs: [comparison with AutoIdle and others](docs/comparison.md) including a migration guide, and an [FAQ](docs/faq.md).
- Rewritten sleep/wake core: the formation is saved before scaling down and never lost on API errors, state changes use compare-and-set updates, apps marked awake with stopped dynos are repaired on the next request, and a reconciler re-checks every app every 5 minutes.
- Web dashboard at `/`: status of every app, next sleep time, wake/sleep/pause buttons, one-click enrolment from a connected Heroku account.
- Savings report (dollars saved, dyno-hours slept, % asleep, monthly projection) from per-process sleep/wake history. `DYNO_HOURLY_PRICES` overrides the price list.
- Pause/resume, web-only mode, optional "waking up" page (`ERROR_PAGE_URL`), configurable night-mode window (`NIGHT_MODE_TZ`, `NIGHT_MODE_START`, `NIGHT_MODE_HOURS`).
- Review apps: `AUTO_ENROLL` globs enrol matching apps automatically; deleted apps are forgotten.
- One-click Heroku deploy (`app.json`), Postgres support alongside MySQL, `REDIS_URL`/`REDIS_TLS_URL` with TLS.
- Prometheus metrics at `GET /metrics` (basic auth): scale events, app errors, rejected drain posts, background jobs, HTTP requests.
- `GET /healthz` liveness probe and `GET /readyz` readiness probe (alias of `/health`).
- `Dockerfile` (static binary on distroless) and `docker-compose.yml` (AutoSleep + Postgres + Redis).
- `LICENSE` (Apache-2.0), `CONTRIBUTING.md`, `SECURITY.md`, issue and pull request templates.
- CI: gofmt, `go mod verify`, vet, race-enabled tests, `govulncheck`, Docker build.

### Changed
- Stored tokens are encrypted with an Argon2id-derived key (v2 envelope). Rows written by earlier versions (MD5-derived key) remain readable and are re-encrypted when rewritten. Keep `PASSPHRASE` unchanged.
- Each Heroku API call uses its own client and token, fixing a race where concurrent jobs could share credentials.
- Go 1.26, gorm v2, current dependency versions.

### Security
- Log drain posts must carry the drain's `Logplex-Drain-Token`; existing apps are backfilled automatically by the reconciler.
- OAuth `state` is a one-time random value in a short-lived cookie instead of the static `STATE_TOKEN`.
- State-changing requests from other sites are rejected; the IP allowlist no longer trusts spoofed `X-Forwarded-For` entries on Heroku.
- The OAuth callback and waking page are rate limited per client IP.

### Removed
- `STATE_TOKEN` env var. It is no longer read.
