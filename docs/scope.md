# Scope — Bookmarks API V1.1

This document defines what V1.1 does and, more importantly, what V1.1 does not do.
Scope is the contract that prevents the project from growing forever.

> Repository: https://github.com/Carlos20052030/bookmarks-api

## Problem

Developers accumulate hundreds of links — articles, docs, repos, tools — in
browser bookmarks, chat messages, and random text files. There is no simple,
self-hosted API to store, tag, and retrieve them with authentication and
ownership, without signing up for a SaaS that owns the data. Furthermore, integrating these bookmarks with other services (like webhooks) requires manual effort.

## Objective

Expose an HTTP API where a user can register, log in, and manage a private
collection of bookmarks with tags. Authenticate with JWT access tokens and revocable refresh tokens. Provide secure API Keys for machine-to-machine integrations. Publish events asynchronously to a Webhook Dispatcher. Support CORS for the Nuntix frontend. Persist in Postgres and deploy via Docker alongside the Nuntix ecosystem.

## In Scope (V1.1)

- User registration with email and password (bcrypt, cost 12, minimum
  password length 8 characters)
- Login returning a short-lived JWT access token (15 min) and an opaque
  refresh token stored in Postgres
- JWT signed with HS256; secret loaded from `.env`, minimum 32 bytes
- Refresh tokens valid for 7 days; rotated on every refresh; the old
  token is revoked immediately
- Refresh token reuse detection — a revoked token presented again
  revokes every active session for that user
- Logout revoking the current refresh token; idempotent for unknown tokens
- Timing attack mitigation in login — a dummy bcrypt verification runs when
  the email is unknown, so both paths take approximately the same time
- **API Keys (Machine-to-Machine):** Generation, listing, and revocation of API Keys. Keys are prefixed with `nuntix_live_`, hashed with SHA-256 before storage, and shown only once during creation.
- **CORS Middleware:** Configurable Cross-Origin Resource Sharing to allow the Nuntix React frontend (localhost and production domains) to communicate with the API.
- **Event Publishing (Fire-and-Forget):** Asynchronous HTTP POST to the Webhook Dispatcher when a bookmark is created.
- Bookmark CRUD scoped to the authenticated user (ownership enforced in
  every SQL query, not only in middleware)
- Tags: create, list, delete; a bookmark may have many tags, a tag belongs
  to one user; `UNIQUE (user_id, name)`
- Attach and detach tags from bookmarks, validating ownership of **both**
  the bookmark and the tag
- Listing with pagination (`limit`, `offset`) and filter by tag
- Middleware chain: CORS, request ID, structured logging, panic recovery,
  authentication, and per-route auth enforcement
- Consistent JSON error responses with a stable shape:
  `{"error": "...", "message": "..."}`
- Routing with `net/http` and Go 1.22+ `ServeMux` patterns
- Postgres 17 via `database/sql` with explicit SQL; migrations as versioned
  `.sql` files applied at startup, embedded with `//go:embed`
- Tests with `httptest` for handlers, hand-written fakes for storage
- Integration tests against a real Postgres (Docker) for storage
- Docker Compose with two services (`api`, `db`) and a named volume
- Multi-stage Dockerfile producing a small runtime image (31 MB)
- Scheduled `pg_dump` backup to a host-mounted directory, retaining the
  last 4 copies, with a documented restore procedure that has been tested
- Rate limiting on `/auth/register` and `/auth/login`, in-memory, per IP,
  token bucket algorithm; returns `429` with a `Retry-After` header
- Best-effort fetch of the page `<title>` on bookmark creation, with a
  5-second timeout and SSRF protection (private, loopback, and link-local
  addresses are refused)
- Deploy on the same Hetzner CX23 VPS, coexisting without disrupting CloudMonitor and sharing the Postgres container with the Dispatcher.

## Out of Scope (V1.1)

These are explicitly deferred. Do not build them in V1.1.

- **Web UI, frontend, SPA, server-rendered HTML:** The frontend lives in the separate `reverse-proxy` (Nuntix) repository. This API only serves JSON.
- OAuth providers (Google, GitHub), magic links, 2FA
- Email verification, password reset flow, email delivery of any kind
- Roles, permissions, admin panel, teams, sharing between users
- Public or shareable bookmark lists
- Nested tags, tag hierarchies, or automatic tag suggestions
- Full-text search inside bookmarked pages
- Scheduled re-fetch of bookmarks, link rot detection, snapshots
- Browser extension, mobile app, CLI client
- Import from browser bookmark exports (HTML, JSON)
- Import from Pocket, Instapaper, Raindrop, or any third party
- Favicon fetching or image proxies
- Notifications, digests, or scheduled emails
- Metrics endpoint (Prometheus), Grafana dashboards, tracing
- **Message queue worker:** The API only publishes events. The actual queue (Redis Streams) and the background workers live in the `webhook-dispatcher` repository.
- Redis, Memcached, or any cache beyond an in-memory rate limiter
- Multi-region, read replicas, sharding, or any HA concern
- Kubernetes, Swarm, Nomad, or any orchestrator beyond Compose
- CI/CD pipelines; deployment is manual via `git pull` + `docker compose`
- API versioning (`/v1/`); V1 is the version
- GraphQL, gRPC, or any protocol other than HTTP+JSON
- Soft delete, audit log, or event sourcing
- Field-level encryption at rest beyond what Postgres and the OS provide
- HTTPS termination via a reverse proxy (handled by the Nuntix Proxy).

## User

A solo developer who wants a self-hosted API to store and retrieve their own
bookmarks with authentication, and who is comfortable running a Docker
Compose stack on a Linux VPS. Secondary user: the Nuntix React frontend
communicating via CORS, and the Webhook Dispatcher receiving event payloads.

## Success Criteria

V1.1 is considered successful when all of the following are true:

1. `go test ./...` passes without failures.
2. `golangci-lint run` reports zero issues.
3. A user can register, log in, refresh, and log out through the API with
   only `curl`, following the README.
4. An authenticated user cannot read, modify, or delete another user's
   bookmark by guessing its ID; the API returns 404, not 403.
5. Refresh token rotation works: the old refresh token is rejected after a
   successful refresh.
6. Presenting a revoked refresh token revokes every active session for that
   user (reuse detection).
7. Access token expiry is enforced; requests with an expired token return
   401 without touching the database beyond the auth middleware.
8. The `api` and `db` containers restart cleanly with
   `docker compose -f docker-compose.prod.yml up -d` after a `down`.
9. `pg_dump` runs on schedule, produces a restorable file, and a documented
   restore has been performed at least once into a scratch database.
10. A stranger can clone the repository and run the stack locally in under
    15 minutes by following the README.
11. No secret (JWT signing key, `POSTGRES_PASSWORD`, `DATABASE_URL`) is ever
    committed to the repository.
12. The service coexists with CloudMonitor on the same VPS without port
    conflicts and without restarting CloudMonitor.
13. The API is reachable from outside the VPS over the public IP.
14. **API Keys** can be generated, listed, and revoked. A request authenticated with a valid API Key succeeds; a revoked key returns 401.
15. **CORS** allows the Nuntix frontend to fetch `/bookmarks` and `/api-keys` successfully.
16. **Fire-and-Forget** publishes an event to the Dispatcher without blocking the HTTP response to the user.

## Constraints

- Build time: 4 weeks, one commit per day (actual: 6 days for V1, +3 days for V1.1).
- No paid third-party services. Postgres runs locally in Docker.
- Target hardware: the existing Hetzner CX23 (2 vCPU, 4 GB RAM), shared
  with CloudMonitor (RSS ~30 MB) and the Dispatcher.
- Language: Go 1.27.
- HTTP layer: `net/http` with `ServeMux` patterns. No router framework.
- Database access: `database/sql` with explicit SQL. No ORM.
- Third-party dependencies limited to: `github.com/jackc/pgx/v5/stdlib`
  (Postgres driver), `github.com/golang-jwt/jwt/v5` (JWT),
  `golang.org/x/crypto/bcrypt` (password hashing), and
  `github.com/google/uuid` (UUID generation).
- Testing: `httptest` and hand-written fakes. No mocking framework.
- Deployment: Docker Compose on the existing VPS. No Kubernetes.
- Postgres is not exposed to the host in production; only the `api`
  container and the `dispatcher` container reach it, over the Compose network.
- Backups go to `/home/carlos/backups/bookmarks/` on the host, not inside
  the Postgres volume.

## Assumptions

- The VPS has Docker Engine and the Compose plugin installed.
- Port 8080 is reachable from the developer's machine for testing; a
  reverse proxy or a direct port is chosen at deploy time.
- The CloudMonitor service is not restarted or disrupted during deployment
  of this project.
- The developer has outbound HTTPS access to fetch dependencies at build
  time.
- A single Postgres instance is enough; no replication or failover.
- Secrets live in `.env`, gitignored, loaded by Compose.

## Version

V1.1 — HTTP+JSON API. API Keys and CORS added. No UI. No OAuth. No sharing. Single instance, single database, single region. Docker Compose is the deployment unit.