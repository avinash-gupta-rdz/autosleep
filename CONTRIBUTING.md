# Contributing to AutoSleep

Thanks for thinking about contributing. AutoSleep gets better when people outside the original author work on it.

## Ground rules

- Be respectful and assume good faith.
- Security issues do **not** belong in public issues. See [SECURITY.md](./SECURITY.md).
- Small, focused PRs beat large ones. If in doubt, open an issue first to agree on scope.

## Dev setup

You need Go (see `go.mod` for the version). Tests need nothing else: they use in-memory SQLite, an in-memory Redis and a fake Heroku API.

```bash
git clone https://github.com/avinash-gupta-rdz/autosleep.git
cd autosleep
go test ./...
```

To click around the dashboard without any services or a Heroku account:

```bash
DEMO_MODE=true go run .  # http://localhost:8080, log in as demo / demo
```

To run the real thing, either use Docker:

```bash
docker compose up        # AutoSleep + Postgres + Redis on http://localhost:8080
```

or your own Postgres/MySQL and Redis:

```bash
cp sample_env_vars .env  # fill in the values — see README.md's "Configuration" section
source .env
go run .
curl -u "$API_USER:$API_PASS" http://localhost:8080/app   # → {"list":[]}
```

## Branching & commits

- Branch from `master`. Name branches `feat/<short-desc>`, `fix/<short-desc>`, `chore/<short-desc>`, etc.
- Write clear commit messages. Reference issues where relevant (`Fixes #42`).

## Code style

- `gofmt -l .` must be empty and `go vet ./...` clean (CI checks both).
- Prefer the standard library over new dependencies unless there's a clear reason.

## Tests

- Tests live next to the code they cover (`foo_test.go` next to `foo.go`).
- New features should come with tests; bug fixes with a regression test.
- Sleep/wake logic is tested against the fake Heroku in `testutil/`; extend it rather than calling the real API.
- `go test -race ./...` must pass before a PR is merged.

## Pull requests

- Rebase on the latest `master` before opening a PR.
- Fill out the PR template — what changed, why, how it was tested.
- Add a line to `CHANGELOG.md` under `## [Unreleased]` for user-visible changes.
- CI must be green before review.

## Questions

Open a [GitHub Discussion](https://github.com/avinash-gupta-rdz/autosleep/discussions) for open-ended questions, or an issue if you've hit something specific.
