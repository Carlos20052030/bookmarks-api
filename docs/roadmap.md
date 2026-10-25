# Roadmap — Bookmarks API

Build plan for the Bookmarks API. Originally planned for 4 weeks (V1), delivered in 6 days. Now expanded to include the SaaS ecosystem integrations (V1.1).

> Repository: https://github.com/Carlos20052030/bookmarks-api

## Overview

- **Planned duration (V1):** 4 weeks (28 days)
- **Actual duration (V1):** 6 days (07/10 to 12/10)
- **Cadence:** at least one commit per day, even if small
- **Definition of done:** `go test ./...` passes, `golangci-lint run` is clean,
  the stack runs under Docker Compose on the VPS next to CloudMonitor, the
  `/health` endpoint is green, backups run on schedule and a restore has been
  proven, and the README lets a stranger run the project in under 15 minutes.
- **Working rule:** do not start the next phase until the previous one runs.

## Phase 1 — Foundation (Week 1)

**Goal:** the project boots, connects to Postgres, and applies migrations.

**Deliverable:** `docker compose up -d db` followed by `go run ./cmd/api`
starts the process, opens a connection pool, applies migrations idempotently,
and logs a startup line.

**Done when:** running migrations twice in a row leaves the schema unchanged.

| Day | Task | Status |
| --- | --- | --- |
| 1 | `go mod init`, directory layout, `.gitignore`, empty `README.md`, `Makefile` | done |
| 2 | `docker-compose.yml` (db service, named volume, healthcheck) + override for dev | done |
| 3 | `internal/domain/` — `User`, `Bookmark`, `Tag`, `RefreshToken` | done |
| 4 | `internal/config/config.go` — load from env, validate, fail fast | done |
| 5 | `migrations/001_init.sql` (users, refresh_tokens) + `internal/storage/migrate.go` | done |
| 6 | `migrations/002_bookmarks.sql`; pool via pgx; `internal/storage/db.go` | done |
| 7 | Server HTTP com `/health` e graceful shutdown; commit | done |

## Phase 2 — Auth (Week 2)

**Goal:** register, login, refresh, logout work end-to-end with JWT.

**Deliverable:** the curl flow in the README succeeds from scratch.

**Done when:** an expired access token returns 401 without hitting the
database beyond the auth middleware, and a reused refresh token is rejected.

| Day | Task | Status |
| --- | --- | --- |
| 8 | `internal/auth/password.go` (bcrypt, cost 12, min 8) + `token.go` (HS256) | done |
| 9 | Middleware: request ID, structured logging, panic recovery | done |
| 10 | `storage/user.go` (Create, GetByEmail) + `storage/refresh.go` (Create, Revoke) | done |
| 11 | `POST /auth/register` with validation and 409 on duplicate email | done |
| 12 | `POST /auth/login` returning access JWT and opaque refresh token | done |
| 13 | Auth middleware (verify JWT, inject user into context) + `POST /auth/logout` | done |
| 14 | `POST /auth/refresh` with rotation and reuse detection; commit | done |

## Phase 3 — Bookmarks, Tags, Middleware (Week 3)

**Goal:** full CRUD with ownership, tags, pagination, and filtering.

**Deliverable:** an authenticated user can create, read, update, delete, and
list their own bookmarks, tagged and paginated, and cannot touch another
user's.

**Done when:** the IDOR test passes (user A gets 404 on user B's bookmark),
and attaching user B's tag to user A's bookmark also returns 404.

| Day | Task | Status |
| --- | --- | --- |
| 15 | `storage/bookmark.go` — Create, GetByID, List, Update, Delete, scoped by `user_id` | done |
| 16 | Handlers: `POST`, `GET /{id}`, `PUT /{id}`, `DELETE /{id}` | done |
| 17 | `GET /bookmarks` with `limit`/`offset` and filter by tag; IDOR test | done |
| 18 | Tags: storage + handlers (create, list, delete), `GET /tags` | done |
| 19 | Attach and detach: storage + handlers; ownership validated on both sides | done |
| 20 | Wire all routes with `requireAuth` per route; end-to-end curl tests | done |
| 21 | Commit; README quickstart drafted | done |

## Phase 4 — Production (V1 Closed)

**Goal:** running in Docker on the VPS next to CloudMonitor, backed up.

**Deliverable:** `docker compose -f docker-compose.prod.yml up -d` on the VPS
brings both services up healthy, `/health` returns 200 over the public IP, and
backups run on schedule.

**Done when:** a stranger can reproduce the setup from the README and
CloudMonitor has not been restarted.

| Day | Task | Status |
| --- | --- | --- |
| 22 | Multi-stage `Dockerfile`, `.dockerignore`, `docker-compose.prod.yml` | done |
| 23 | Install Docker Engine on the VPS; generate production secrets; deploy | done |
| 24 | Test the API from outside the VPS; confirm coexistence with CloudMonitor | done |
| 25 | `pg_dump` backup script + cron entry + restore test | done |
| 26 | README completo: badges, quickstart, curl examples, endpoints | done |
| 27 | `docs/architecture.md`, `docs/deployment.md`, `docs/postmortem.md` | done |
| 28 | Push final; declare V1 closed | done |

## Phase 5 — SaaS Integration & API Keys (V1.1)

**Goal:** Integrate the Bookmarks API into the Nuntix Monorepo ecosystem, allowing the React frontend to communicate with it and external services to authenticate via API Keys.

**Deliverable:** A secure API endpoint to manage API Keys and a CORS middleware to allow the Nuntix UI to fetch data.

| Task | Status |
| --- | --- |
| `internal/middleware/cors.go` to allow the Nuntix frontend (localhost and prod) | done |
| `internal/dispatcher/client.go` to publish events (Fire-and-Forget) | done |
| `migrations/003_api_keys.sql` (hash, unique, indices) | done |
| `internal/domain/api_key.go` & `internal/storage/api_key.go` | done |
| `internal/handler/api_key.go` with SHA-256 generation and raw key return | done |
| Wire routes in `server.go` and `main.go` | done |

## Milestones

| Milestone | End of  | What it proves |
| --- | --- | --- |
| M1 | Week 1 | Project boots, migrations are idempotent, pool connects |
| M2 | Week 2 | Register/login/refresh/logout work with JWT |
| M3 | Week 3 | CRUD + tags + ownership + reuse detection all work; tests green |
| M4 | Week 4 | Docker Compose runs on the VPS, backups tested, README ready |
| M5 | V1.1 | CORS working, API Keys endpoints exposed for the Nuntix UI |

## Risks

| Risk | Mitigation |
| --- | --- |
| JWT confusion between access and refresh | Separate packages and separate tests; access is JWT, refresh is opaque |
| IDOR bugs (other users' rows leaking) | Every SQL query scoped by `user_id`; dedicated tests prove 404, not 403 |
| Migrations not idempotent | Run the migration runner twice in a test before shipping |
| Postgres password caching in the volume | Documented in `deployment.md`; reset procedure uses `down -v` |
| Rate limiter state lost on restart | Accepted for V1; deferred to V1.1 |
| Port conflict with CloudMonitor | Verified before deploy; API uses 8080, CloudMonitor uses none |
| Deploy disturbing CloudMonitor | Never `systemctl restart cloudmonitor`; verification after every step |
| Docker bypasses ufw silently | Documented in `deployment.md` troubleshooting section |
| PyCharm save race before `git add` | Always `Ctrl+S`; create new files from the terminal when in doubt |

## Post-V1 (V2 and beyond)

- **Redis-backed rate limiter:** Survives container restart and is shared across replicas.
- **HTTPS termination via Caddy:** Requires a domain name (now solved by Nuntix's autocert).
- **Prometheus metrics and Grafana dashboards:** To monitor the API health and throughput.
- **Multi-tenant Webhooks:** Allow users to register webhook endpoints directly from the Nuntix UI (currently handled by the Dispatcher).
- **CI/CD pipeline:** Replacing manual `git pull` + `docker compose` with GitHub Actions.