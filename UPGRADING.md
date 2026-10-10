# Upgrading AutoSleep

The dashboard footer shows the version you run and tells you when a newer
release is out (AutoSleep checks GitHub once a day; set `UPDATE_CHECK=false`
to turn that off). Release notes are in [CHANGELOG.md](CHANGELOG.md).

Upgrades are safe to apply at any time:

- Database changes run automatically at start-up and only **add** tables
  and columns, so your apps, history and savings are kept, and the previous
  version still runs against the upgraded database if you need to roll back.
- Managed apps are not touched by the deploy. While AutoSleep restarts
  (a few seconds) a visit to a sleeping app may not wake it; the next
  visit does.
- **Never change `PASSPHRASE`.** Stored Heroku keys are encrypted with it.

## Installed with the Deploy button

The Deploy button copies the code once and doesn't follow later releases.
Pick one of these ways to update; the first is set-and-forget.

### Option A: automatic deploys from your fork (recommended)

1. [Fork the repository](https://github.com/avinash-gupta-rdz/autosleep/fork) on GitHub.
2. In the Heroku dashboard open your AutoSleep app → **Deploy** →
   **Deployment method: GitHub**, connect your fork and click
   **Enable Automatic Deploys** for the `master` branch.
3. To upgrade, click **Sync fork** on your fork's GitHub page. Heroku deploys
   it within a minute or two.

### Option B: from your terminal

```sh
git clone https://github.com/avinash-gupta-rdz/autosleep && cd autosleep
heroku git:remote -a <your-autosleep-app>
git push heroku master
```

Next time, `git pull` then `git push heroku master` again.

## Installed from a git clone

```sh
git pull
git push heroku master
```

## Docker

Rebuild the image from the new source (`docker compose build` or
`docker build -t autosleep .`) and restart the container. The data lives
in your database, not the container.

## Before a big jump

Optional but cheap: take a database backup first.

```sh
heroku pg:backups:capture -a <your-autosleep-app>
```

If something looks wrong after an upgrade, `heroku rollback -a <your-autosleep-app>`
returns to the previous release, and please
[open an issue](https://github.com/avinash-gupta-rdz/autosleep/issues/new/choose).
