# FAQ

## How long does a visitor wait when an app is asleep?

Usually 10–30 seconds: the time Heroku needs to start the dynos and your app
needs to boot. Meanwhile visitors see AutoSleep's "waking up" page (on by
default; it sets the app's `ERROR_PAGE_URL`). If an app must never make
anyone wait during working hours, give it **office hours** in Settings.

## My app has WebSockets or long-lived connections. Will it fall asleep under active users?

It can. AutoSleep sees traffic through Heroku's router logs, and the router
logs a WebSocket (or other long-lived) connection once, when it opens.
Messages sent over an open socket produce no log lines. An app whose users
only keep a socket open, without new HTTP requests, looks idle.

Use a longer idle timeout, office hours, or **Pause** for such apps.

## Does it work with worker-only apps?

AutoSleep wakes apps on HTTP requests, so an app with no web process is never
woken by traffic. Use office hours or the **Wake now** button for those, or
turn on **Web only** for apps whose workers must keep running.

## <a id="fir"></a>Does it support Heroku Fir (the next generation)?

Not yet. Fir apps can only send logs as OpenTelemetry "telemetry drains";
the HTTPS log drains AutoSleep relies on are available on Cedar only
([Heroku: Generations](https://devcenter.heroku.com/articles/generations),
[Log Drains](https://devcenter.heroku.com/articles/log-drains)). Fir is
currently generally available only in Private Spaces. Supporting it needs an
OpenTelemetry receiver and a check that a sleeping Fir app still reports
incoming requests. Cedar apps, including those in the Common Runtime, are
fully supported.

## Private Spaces?

AutoSleep scales Private Space apps like any other, but it hasn't been
verified there yet. Heroku's routing in Private Spaces differs in places
(for example, [maintenance mode](https://devcenter.heroku.com/articles/maintenance-mode)
pages aren't shown when no web dynos run), so visitors may get an error
instead of the waking page. Test with one app first, and please report what
you see.

## Eco dynos?

Eco dynos already sleep after 30 idle minutes and wake on request, for free.
AutoSleep can manage them, but it won't save money.

## What happens if AutoSleep itself is down?

Sleeping apps stay asleep until AutoSleep is back (they are woken on the
next visit after that, or with **Wake now**). Awake apps keep running.
Nothing is lost: saved formations are in AutoSleep's database. Run AutoSleep
on a Basic or larger dyno, never on Eco.

## What can AutoSleep do with my Heroku key?

Everything the key's account can do, so treat it like a password. AutoSleep
only reads formations and apps, scales formations, manages its own log
drains and sets `ERROR_PAGE_URL` when you enable the waking page. Keys are
stored encrypted with `PASSPHRASE`. To limit exposure, create the key from a
Heroku account that is only a collaborator on the non-production apps.

## Something isn't working

Click **Check** next to the app. It tests Heroku access, the log drain and
whether logs are arriving, and explains each problem; most can be fixed with
one click. If not, please
[open an issue](https://github.com/avinash-gupta-rdz/autosleep/issues/new/choose)
with the Check output.
