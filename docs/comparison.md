# AutoSleep compared

Several Heroku add-ons and scripts cut the cost of apps nobody is using.
They solve slightly different problems; this page tries to be fair about
where each one wins. Prices are from the Heroku Elements listings and Dev
Center articles as of October 2026; check the linked pages for current ones.

| | AutoSleep | [AutoIdle](https://devcenter.heroku.com/articles/autoidle) | [FlightFormation](https://devcenter.heroku.com/articles/flightformation) | [Judoscale](https://elements.heroku.com/addons/judoscale) | Cron / Heroku Scheduler script |
|---|---|---|---|---|---|
| Sleeps an app when idle | Yes | Yes | No (fixed schedule) | Workers only | No (fixed schedule) |
| Wakes on the next request | Yes | Yes | No | No | No |
| Keep awake during office hours | Yes | Not documented | Yes (schedules) | Yes (schedules) | Yes |
| First visitor sees | A "waking up" page on your own domain, 10–30 s | An error on `*.herokuapp.com`; no error via AutoIdle's proxy domain or a CNAME to it | An error while "off" | n/a | An error while "off" |
| Review apps | Name pattern (`AUTO_ENROLL`), cleaned up on delete | `app.json` add-on entry per app | Per app | Per app | Per app |
| Savings report | Yes | Yes | No | Testimonials only | No |
| Alerts | Slack / Discord / webhook, weekly digest | Not documented | Not documented | — | Your own |
| Code changes | None | None (domain change for best experience) | None | Agent library | Script |
| Heroku Fir generation | Not yet (see [FAQ](faq.md#fir)) | Yes | Yes | Yes | Yes |
| Open source / self-hosted | Yes (Apache-2.0) | No | No | No | Yours |
| Price | ~$15/month total in your own Heroku resources, any number of apps | Per app per month: Basic $4, Standard $16, Performance $100, Private Spaces $150 (web-only plans half) | From $12/month per app after a 7-day trial | Free plan with 20 scale events/month; paid from $20/month | Scheduler dyno time |

## When to pick something else

- **One small app on a Basic dyno.** AutoIdle's $4 plan is cheaper than
  running AutoSleep (~$15/month). AutoSleep pays off from a few apps, or
  from one Standard app.
- **Eco dynos.** They already sleep after 30 idle minutes and wake on
  request, for free. Neither AutoSleep nor AutoIdle saves money there.
- **Production apps under real load.** You want an autoscaler such as
  Judoscale, not a sleeper.
- **Apps on Heroku Fir.** AutoSleep can't see their traffic yet.
- **You never want a first visitor to wait.** Use office hours (AutoSleep or
  FlightFormation) and accept paying for the hours the app is kept awake.

## What a fleet costs

Ten Standard-1X staging and review apps:

- AutoIdle: 10 × $16 = up to **$160/month** in add-on fees.
- AutoSleep: **about $15/month** (Basic dyno $7 + Postgres Essential-0 $5 +
  Key-Value Store Mini $3), the same for 1 app or 100.

Both save the same dyno-hours; AutoSleep's dashboard shows how much.

## Moving from AutoIdle

1. Deploy AutoSleep and connect your Heroku account (see the
   [README](../README.md#set-up-for-real-about-5-minutes)).
2. For each app, remove the add-on, then enable the app in AutoSleep:
   ```sh
   heroku addons:destroy autoidle -a my-staging-app
   ```
   Do it while the app is awake, so its formation is running when AutoSleep
   saves it. Removing AutoIdle deletes its data, including its savings
   history.
3. Review apps: delete the `autoidle` entry from `environments.review.addons`
   in `app.json` and set `AUTO_ENROLL` to your review-app name pattern
   instead (e.g. `myapp-pr-*`).
4. If visitors used `*.autoidleapp.com` or a CNAME to `ssl.autoidleapp.com`,
   point them back at the app's own domain; AutoSleep's waking page works
   there.
5. AutoIdle's `idling-time` maps to AutoSleep's idle timeout (Settings, or
   `AUTO_ENROLL_IDLE_MINUTES` for review apps). Its web-only plans map to
   the **Web only** setting.
