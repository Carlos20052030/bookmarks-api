# Postmortem — Bookmarks API V1.1

Written on 12/10/2026, updated on 25/10/2026.

This is not a list of wins. It is what actually happened, what went wrong,
and what I would do differently next time.

> Repository: https://github.com/Carlos20052030/bookmarks-api

## Timeline

- **Day 1 (07/10)** — repo, layout, Docker Compose, Postgres up
- **Day 2 (08/10)** — domain, config, storage, migrations, HTTP server
- **Day 3 (09/10)** — bcrypt, JWT, refresh tokens, middleware
- **Day 4 (10/10)** — auth handlers, bookmark CRUD, IDOR tests
- **Day 5 (11/10)** — tag handlers, multi-stage Dockerfile, deploy to VPS
- **Day 6 (12/10)** — README, architecture guide, backup script, deployment doc
- **Phase 5 (17/10 to 25/10)** — Webhook Dispatcher integration, Nuntix Proxy (TLS), API Keys, and Database Consolidation.

## What went well

**Tests before curl.** Every handler, every repository, every middleware got
unit tests before it ever touched a real HTTP request. By the time I opened
`curl`, I was not debugging features — I was verifying them. The first
production smoke test passed on the first attempt because the code had
already been exercised 130+ times.

**Ownership scoping in SQL, not in middleware.** Every query filters by
`user_id`. The IDOR tests (`TestGetBookmarkByID_IDOR`, `TestUpdateBookmark_IDOR`,
`TestDeleteBookmark_IDOR`) are the same three tests written three times, and
they are the most valuable tests in the repository. They caught the design
before it became a bug.

**The dependency graph stayed visible.** `cmd/api/main.go` builds every
dependency and injects it. No global singletons, no `init()` magic, no
hidden wiring. When something broke at 2 a.m. (it did), I could trace
exactly what depended on what.

**The multi-stage Dockerfile worked on the first build.** 800 MB builder,
31 MB runtime, `CGO_ENABLED=0`, non-root user, healthcheck. The image
deploys to a 4 GB VPS without a single adjustment.

## What went wrong

**Secrets, twice.**

1. On Day 2 I pasted the local Postgres password into a chat window while
   asking for help with `.env` syntax. It was a dev-only password against a
   local Docker container, so the blast radius was zero. But it was still
   a mistake. Fix: rotate before deploy (done), and never paste values, only
   names (`POSTGRES_PASSWORD=...`).

2. On Day 5 I realized the Telegram bot token for CloudMonitor had leaked
   in a *different* project's chat a few days earlier. That one was a real
   leak — the token was live. Fix: `/revoke` in BotFather, new token. Since
   then, secrets live only in KeePassXC.

**The PyCharm save race.**

Twice, PyCharm wrote files to disk *after* `git add` ran. The first time,
two files were committed empty and I had to rewrite history with
`git reset --soft`. The second time, an `internal/store.go` appeared at the
wrong level of the tree — duplicated content in two places. The fix was
not obvious: `Ctrl+S` before every `git add`, and create new files from
the terminal when in doubt. Two wasted hours across both incidents.

**Docker bypasses ufw silently.**

After deploy, `curl http://<vps-ip>:8080/health` worked from my laptop
before I ever ran `sudo ufw allow 8080`. Docker inserts its own iptables
rules before `ufw` sees the traffic. The `ufw status` output still shows
only ports 22, 80, and 443 — but 8080 is open. This is documented Docker
behavior, not a bug, but it is not obvious until you hit it.

**Pasted content into the terminal.**

The shell tried to execute every line of a Mermaid block (and later a markdown block) as a command.
It created five empty files named `DB`, `Handlers`, `MW`, `Server`, and
`Storage`. Removed with `rm`. Lesson: long text goes into the editor,
not the terminal. Terminals interpret; editors accept.

**The IDE did not support Go.**

For the first six days of the project I was using PyCharm Community as
the editor. It rendered Go files, saved them on `Ctrl+S`, and let me run
`go build` and `go test` in the integrated terminal. What I did not
realize is that PyCharm Community **does not support the official Go
plugin** — JetBrains reserves it for PyCharm Professional and GoLand.
The IDE was treating `.go` files as plain text. No autocomplete, no
jump-to-definition, no smart quote pairing.

I discovered this on day 7 when the auto-close-quote feature stopped
working. Three hours of troubleshooting later (Invalidate Caches, moving
config folders, reinstalling PyCharm), the root cause was structural,
not a bug: the editor had never supported the language.

Fix: migrated to GoLand (trial), installed the official Go 1.27.1 from
`go.dev` (replacing the Snap install), and configured Go Modules with
GOPATH. Imports resolved. Everything works.

**Flapping notification from CloudMonitor targets.**

Between 08/10 and 13/10 the CloudMonitor sent several DOWN/UP alerts
alternating for Google and Example. Diagnosed as transient route
problems between the Hetzner datacenter and the target CDNs. Ping tests
showed 0% packet loss and 3-7ms latency when checked. The service itself
never crashed — the streak of 7 consecutive days was not affected.

## What surprised me

**The multi-stage image was 31 MB.** The builder was 800 MB. I knew it
would be smaller, but the ratio (26x) was still striking. It changes how
you think about deploys.

**`apt` told me "No services need to be restarted" after installing Docker.**
That one line was worth more than any deployment guide. It confirmed that
installing new packages on a VPS does not disturb running services — a
non-obvious fact that gave me confidence to install Docker on a machine
already running a production service (CloudMonitor, at the time).

**Postgres caches the user password on first init.** Changing
`POSTGRES_PASSWORD` in `.env` and restarting does nothing. The password
lives in the volume. Fix requires `docker compose down -v` (which deletes
data). This cost me an afternoon of "why does the password not work".

**bcrypt's salt is invisible.** Two hashes of the same password are
different strings. The test `TestHashPassword_DifferentSaltsPerCall`
asserts this — but I only understood *why* it mattered after writing it.
Salts defeat rainbow tables. The cost is 60 extra bytes per row. Trivial.

## What I would do differently

**Put the backup script earlier.** `scripts/backup.sh` came on Day 6. The
API was in production on Day 5. For about 20 hours there was no backup.
Nothing bad happened, but the window was unnecessary. Backups belong
in the same PR as the database schema.

**Separate dev and prod credentials from Day 1.** I generated production
secrets on Day 5 and realized I had been reusing the local ones by habit.
Segregation now exists, but it should have been an early decision, not a
late realization.

**Cut the number of commits per feature.** I wrote one commit per logical
change, but some of them ("docs: add architecture guide") bundled 340
lines. Smaller commits would have made the review easier and the history
more readable.

**Do not paste long content into a terminal.** Ever again. Long text into the IDE.
Commands into the terminal. Simple rule, learned twice.

## What I learned

- **Interface defined by the consumer.** The `handler` package declares
  the `Store` interface it needs; `storage` implements it without knowing
  who consumes it. This is the single most idiomatic Go pattern I picked
  up, and it made testing trivial — hand-written fakes, no mock library.
- **Multi-stage Dockerfiles are not advanced, they are standard.** Two
  `FROM` lines, a `COPY --from=builder`, and a 26x reduction in image
  size. There is no reason to ship a Go binary without this.
- **`//go:embed` for migrations.** The binary carries its own `.sql` files.
  No runtime filesystem dependency. The deploy is one file.
- **Ownership scoping is a design decision, not a check.** Filtering by
  `user_id` in every query is what makes IDOR structurally impossible, not
  an `if` in a middleware.
- **Timing attacks are real, not theoretical.** The `TestLogin_UnknownEmail`
  test takes ~200 ms because a dummy bcrypt call runs. Without that call,
  it would return in microseconds — and an attacker could measure the
  difference.
- **`404` not `403`.** Leaks less. Everyone should know this; almost no
  tutorial says it.

## On using AI as a pair programmer

The code in this repository was written by me, but it was not written alone.
I used an AI assistant throughout the project — to explain how `http.ServeMux`
patterns work in Go 1.22, to walk through multi-stage Dockerfiles, to
sanity-check my SQL queries, and to catch a few decisions I had not thought
through (e.g. the difference between returning `403` and `404` for another
user's resource).

I do not consider this a weakness. Modern development is not "write every
character yourself" — it is "know what you are writing and why". The AI
answered questions; it did not write features. Every commit, every test,
every deployment decision was mine, and I can defend each one in an
interview.

What would be a weakness is using AI to *avoid* learning. The four-hour
debugging session that ended with "Postgres caches the password in the
volume" was me. The AI answered none of that. It just told me where to
look.

---

## Addendum 1: Webhook Dispatcher Integration (17/10/2026)

Five days after V1 went live, the API was extended to publish events
to a separate Webhook Dispatcher service. This section documents the
issues found during that integration.

**Docker networking: `host.docker.internal` does not work on Linux.**

The Bookmarks API could not reach the Webhook Dispatcher. Logs showed
`dial tcp: lookup host.docker.internal: no such host`.
`host.docker.internal` is a Docker Desktop feature (Mac/Windows). On
native Linux, Docker does not resolve it by default.
**Fix:** Added `extra_hosts: - "host.docker.internal:host-gateway"` to
the API's `docker-compose.yml`. This maps the hostname to the Docker
bridge gateway, allowing the container to reach the host.
**Lesson:** Always test container networking on the target OS.

**Port conflict (8080).**
Running both the API and the Dispatcher locally caused a `port is
already allocated` error.
**Fix:** Mapped the Bookmarks API to host port 8081 (`"8081:8080"` in
Compose) while keeping the internal container port at 8080.
**Lesson:** Container ports and host ports are independent.

**JWT expired during manual integration testing.**
Creating a bookmark returned `401 Unauthorized` even with a valid token.
**Root cause:** The JWT access token has a 15-minute TTL. By the time the `curl` was executed, it had expired.
**Fix:** Regenerated the token via `POST /auth/login` immediately before testing.

---

## Addendum 2: Nuntix Monorepo & API Keys (25/10/2026)

As the ecosystem grew, the API was integrated into the Nuntix ecosystem. The `reverse-proxy` (built in Go + React) now acts as the entry point, serving the frontend and routing traffic to this API. This required adding CORS and a new authentication method for machine-to-machine communication (API Keys).

**The Abuse Report (Security Incident).**

We received an alert from Hetzner/CERT-Bund indicating that our Redis container (used by the Dispatcher) was exposed to the internet.
**Root cause:** Docker ignores UFW rules by default. The compose file had `ports: - "6379:6379"`, exposing it to `0.0.0.0`.
**Fix:** Changed all compose files to map ports to `127.0.0.1` (e.g., `127.0.0.1:6379:6379`) and ran `sudo ufw deny 6379`.
**Lesson:** Docker's port mapping bypasses UFW. Always bind sensitive ports to localhost in production.

**Database Consolidation (The end of over-engineering).**

We realized the Bookmarks API and Dispatcher were running separate Postgres containers, wasting RAM and violating the `scope.md` we wrote.
**Fix:** We dumped the Dispatcher database, restored it inside the Bookmarks container (`bookmarks-api-db-1`) as a separate logical database (`webhook_dispatcher`), and deleted the redundant container.
**Lesson:** In a single-node architecture, Shared Database (logical isolation) is preferred over separate containers for cost and maintenance simplicity.

**The API Keys integration.**

The frontend (React) could not authenticate using JWTs in external scripts. We added API Keys (hashed with SHA-256) to allow server-to-server communication.
**Problem:** The React frontend was getting CORS errors when trying to fetch `/api-keys`.
**Fix:** We added a `cors.go` middleware and ensured `Access-Control-Allow-Origin` was set for `nuntix.com.br`.

**CORS and the Monorepo.**

When we moved the React frontend to be served directly by the Go Proxy (Monorepo), we no longer needed CORS for the frontend. However, we kept it for local development (`localhost:5173`) and as a safety net.
**Lesson:** Monorepos eliminate CORS in production. If both frontend and backend live in the same binary, the browser sees them as the same origin.

## Thanks

To everyone who reviewed a broken commit, explained what `alpine` is, and
told me to stop pasting into the terminal. You know who you are.