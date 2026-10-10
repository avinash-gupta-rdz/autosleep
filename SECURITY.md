# Security policy

AutoSleep stores third-party API tokens (Heroku) and controls the scaling of users' apps. We take vulnerabilities seriously.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report privately via one of:

1. **Preferred — GitHub Security Advisories:** open a draft advisory at https://github.com/avinash-gupta-rdz/autosleep/security/advisories/new
2. Email: `atechnodrifter@gmail.com` with subject `[autosleep security]`

Please include a description of the issue and its impact, steps to reproduce (or a proof of concept), the affected version or commit, and any suggested fix.

## What to expect

- **Acknowledgement** within 3 business days.
- **Initial assessment** within 7 business days.
- Regular updates while we work on a fix.
- Credit in the release notes once the fix ships, unless you prefer to remain anonymous.

## Supported versions

AutoSleep is pre-1.0. Only the latest release (and `master`) receive security fixes.

## Scope

In scope: code in this repository and the default configuration as shipped (including `app.json`, the `Dockerfile` and `docker-compose.yml`).

Out of scope:

- Vulnerabilities in dependencies — please report those upstream; we bump versions and run `govulncheck` in CI.
- Misconfigurations by self-hosters (e.g. a trivial `API_PASS` or `PASSPHRASE`). If you find a footgun in the defaults, please report it as a bug.
- Social-engineering attacks against project maintainers.

## How secrets are protected

- Heroku API keys and OAuth tokens are encrypted at rest with AES-256-GCM, using a key derived from `PASSPHRASE` with Argon2id and a random salt. Rows written by older versions (MD5-derived key) are still readable and are re-encrypted when rewritten.
- Each Heroku API call uses its own client and token; no credentials are shared between concurrent jobs.
- Log drain posts must carry the drain's `Logplex-Drain-Token`, compared in constant time.
- The OAuth `state` is a one-time random value bound to a short-lived, HTTP-only cookie.
- The dashboard and API require HTTP basic auth, can be restricted to IP ranges (`WHITELISTED_IPS`), and reject cross-site state-changing requests.
