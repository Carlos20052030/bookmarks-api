# Architecture — Bookmarks API V1.1

> Repository: https://github.com/Carlos20052030/bookmarks-api

How the service is put together, why each layer exists, and which decisions
were made along the way.

## 1. Overview

Bookmarks API is a single Go binary that speaks HTTP, talks to Postgres, and
does nothing else. It has no framework, no ORM, and no runtime dependency
beyond a Postgres connection and its own configuration.

```mermaid
flowchart TD
    Client[HTTP Client / React Frontend]
    Server[net/http Server]
    MW["Middleware<br/>CORS → request-id → logging → recovery → auth"]
    Handlers["Handlers<br/>auth · bookmark · tag · api-key"]
    Storage["Storage<br/>typed repositories<br/>scoped by user_id"]
    DB[(Postgres)]
    Disp[Webhook Dispatcher]

    Client --> Server --> MW --> Handlers --> Storage --> DB
    Handlers -.-> Disp
```

## 2. Design Principles

1. **Single responsibility per package.** Every `internal/` package does one
   thing. `handler` never imports `storage`; it depends on an interface.

2. **Dependencies point inward.** `domain` depends on nothing. Everything
   else may depend on `domain`. Interfaces live where they are consumed,
   never where they are implemented.

3. **Standard library first.** `net/http`, `database/sql`, `log/slog`,
   `context`, `crypto/rand`. Third-party code only where the standard
   library cannot reasonably do the job.

4. **Fail loud, exit clean.** Startup errors exit non-zero with a structured
   log line. Runtime errors on a single request return a stable JSON body
   and never crash the process.

5. **No global state.** Every dependency is passed explicitly from
   `cmd/api/main.go`. Nothing is a package-level singleton.

6. **Ownership is enforced in SQL.** Not in middleware, not in handlers. The
   database query itself filters by `user_id`. If the filter is removed by
   accident, the IDOR tests break.

## 3. Component Responsibilities

| Package | Responsibility | Depends on |
|---|---|---|
| `cmd/api` | Read config, open DB, run migrations, wire handlers, start server | all internal packages |
| `internal/domain` | Shared types: `User`, `Bookmark`, `Tag`, `RefreshToken`, `APIKey` | nothing |
| `internal/config` | Load and validate environment variables | `os` |
| `internal/auth` | Password hashing, JWT issue/parse, refresh token generation | `crypto/*`, `golang-jwt` |
| `internal/storage` | Postgres repositories, connection pool, migration runner | `database/sql`, `pgx`, `domain` |
| `internal/middleware` | Cross-cutting concerns: CORS, request ID, logging, panic recovery, auth | `net/http`, `auth` |
| `internal/handler` | HTTP handlers: parse request, call storage, write response | `storage` (via interfaces), `domain`, `auth` |
| `internal/server` | Compose the middleware chain and routes, own the `http.Server` | `net/http`, `handler`, `middleware` |
| `internal/dispatcher` | Fire-and-forget client to send events to the Webhook Dispatcher | `net/http` |

## 4. Domain Model

```go
type User struct {
ID           uuid.UUID
Email        string
PasswordHash string
CreatedAt    time.Time
UpdatedAt    time.Time
}

type Bookmark struct {
ID        uuid.UUID
UserID    uuid.UUID
URL       string
Title     string
Tags      []Tag
CreatedAt time.Time
UpdatedAt time.Time
}

type Tag struct {
ID        uuid.UUID
UserID    uuid.UUID
Name      string
CreatedAt time.Time
}

type RefreshToken struct {
ID        uuid.UUID
UserID    uuid.UUID
TokenHash string
ExpiresAt time.Time
RevokedAt *time.Time
CreatedAt time.Time
}

type APIKey struct {
ID        uuid.UUID
UserID    uuid.UUID
Name      string
KeyPrefix string
KeyHash   string
CreatedAt time.Time
RevokedAt *time.Time
}
```

Three invariants hold across all packages:

- **IDs are UUIDv4.** Sequential integers would leak information (how many
  records exist, which IDs are valid). UUIDs make enumeration impractical.
- **`RefreshToken` stores the hash, never the plaintext.** A leaked database
  does not leak usable tokens.
- **`APIKey` stores the hash, never the plaintext.** API keys are generated
  with high entropy, hashed with SHA-256, and shown to the user only once.

## 5. Request Lifecycle

Every HTTP request goes through the same chain:

1. **`middleware.CORS`** — Allows the React frontend (localhost and production domains) to make requests to the API.
2. **`middleware.RequestID`** — Generates a UUIDv4, stores it in the request
   context, echoes it in the `X-Request-Id` response header.
3. **`middleware.Logging`** — Records method, path, status, bytes, duration
   and the request ID on a single line, after the response is written.
4. **`middleware.Recovery`** — Catches panics, logs the stack, replies
   `500 internal_server_error`, keeps the process alive.
5. **Route match** — `net/http` `ServeMux` with Go 1.22+ patterns
   (`GET /bookmarks/{id}`).
6. **`middleware.Auth`** (protected routes only) — Validates the Bearer token,
   injects the user ID into the context. On any failure returns `401`.
7. **Handler** — Parses JSON, validates input, calls a `storage` method,
   writes a JSON response.

The order matters. CORS must be first so preflight (OPTIONS) requests pass without needing authentication. Logging outside Recovery so a panic still produces a log line with `status=500`.

## 6. Authentication Flows

### 6.1. JWT & Refresh Token Flow

#### Registration

```text
POST /auth/register {email, password}
    |
    +-- normalize email (lowercase + trim)
    +-- validate email shape and password length
    +-- bcrypt(password, cost 12)
    +-- INSERT INTO users ...
    +-- issue JWT access token (HS256, 15 min)
    +-- generate refresh token (32 random bytes -> base64url)
    +-- INSERT INTO refresh_tokens (hash only)
    +-- 201 {user, access_token, refresh_token}
```

#### Login

```text
POST /auth/login {email, password}
    |
    +-- normalize email
    +-- SELECT user by email
    |       +-- if not found: run bcrypt against a dummy hash,
    |           return the same 401 as a wrong password
    +-- bcrypt.CompareHashAndPassword
    |       +-- if mismatch: 401
    +-- issue new JWT + new refresh token
    +-- 200 {user, access_token, refresh_token}
```

The dummy bcrypt call is deliberate: without it, a login for an unknown
email would return in milliseconds and a login for a known email would
return in ~200 ms. That timing difference lets an attacker enumerate
registered accounts. Running bcrypt on a hard-coded hash makes both
paths take approximately the same time.

#### Refresh

```text
POST /auth/refresh {refresh_token}
    |
    +-- hash the incoming token (SHA-256)
    +-- SELECT refresh_token by hash
    |       +-- if not found: 401
    |       +-- if revoked:  REUSE DETECTED
    |       |                revoke every active refresh token
    |       |                for that user, return 401
    |       +-- if expired: 401
    +-- revoke the presented token
    +-- issue new access + refresh pair
    +-- 200 {access_token, refresh_token}
```

Reuse detection is the most important part. If the same refresh token
appears twice, one of two things happened: the client raced itself, or
the token was stolen. Neither is safe, so every session for that user
is revoked. The attacker gains nothing, and the legitimate user sees
their other sessions close — a visible signal that something is wrong.

### 6.2. API Key Flow (Machine-to-Machine)

While JWTs are used for user sessions, API Keys are used for server-to-server integrations (e.g., scripts, CLIs, or the Webhook Dispatcher).

#### Generation

```text
POST /api-keys {name}
    |
    +-- generate 32 random bytes (crypto/rand)
    +-- rawKey = "nuntix_live_" + hex(bytes)
    +-- hash = SHA-256(rawKey)
    +-- INSERT INTO api_keys (user_id, name, key_prefix, key_hash)
    +-- 201 {key_metadata, raw_key} // raw_key is returned ONLY ONCE
```

The raw key is never stored in the database. Only the SHA-256 hash is persisted. If the database is compromised, the attacker cannot use the keys. The `nuntix_live_` prefix is used to help secret scanners (like GitHub's) identify leaked keys.

#### Authentication

When a request arrives with an API Key instead of a JWT:

```text
Authorization: Bearer nuntix_live_abc123...
    |
    +-- extract token from header
    +-- hash = SHA-256(token)
    +-- SELECT user_id FROM api_keys WHERE key_hash = hash AND revoked_at IS NULL
    |       +-- if not found: 401 Unauthorized
    +-- inject user_id into the request context
    +-- proceed to the handler
```

## 7. Ownership Model

Every bookmark, tag, and API key belongs to exactly one user. Every read,
write, and delete is scoped by `user_id` at the SQL level:

```sql
SELECT ... FROM bookmarks WHERE id = $1 AND user_id = $2
DELETE   FROM api_keys  WHERE id = $1 AND user_id = $2
```

When the filter does not match, the query returns zero rows. The storage
layer translates that into `ErrNotFound`, and the handler returns `404`.

**Why `404`, not `403`.** A `403` would confirm the resource exists. A `404`
is indistinguishable from "no such resource", which leaks nothing about
other users' data.

## 8. Storage Layer

`storage.Store` wraps a `*sql.DB` and exposes typed methods. Every method
takes a `context.Context` for cancellation.

The migration runner reads `.sql` files embedded in the binary via
`//go:embed`, applies any not yet recorded in `schema_migrations`, and
runs each one inside its own transaction.

Connection pool limits:
- `MaxOpenConns`: 10
- `MaxIdleConns`: 5
- `ConnMaxLifetime`: 30 min

## 9. Security Decisions

| Decision | Why |
|---|---|
| UUIDv4 IDs | Prevent enumeration; make guessing impractical |
| `404` instead of `403` | Avoid leaking resource existence |
| bcrypt cost 12 for passwords | Modern recommendation; ~200 ms per hash |
| SHA-256 for refresh tokens & API keys | High entropy tokens; bcrypt only adds latency |
| API Key prefix (`nuntix_live_`) | Allows secret scanners (like GitHub) to detect leaks |
| HS256 JWT with `WithValidMethods` | Blocks algorithm confusion attacks |
| Timing-safe login (dummy bcrypt) | Prevents user enumeration |
| Refresh token rotation | Limits the window of a stolen token |
| CORS validation | Prevents malicious websites from making requests on behalf of the user |

## 10. Directory Layout

```text
bookmarks-api/
|-- cmd/api/                    main.go — process entry point
|-- internal/
|   |-- auth/                   bcrypt, JWT, refresh token helpers
|   |-- config/                 env loading and validation
|   |-- domain/                 shared types (User, Bookmark, Tag, APIKey)
|   |-- dispatcher/             fire-and-forget HTTP client for webhooks
|   |-- handler/                HTTP handlers (auth, bookmark, tag, api_key)
|   |-- middleware/             cors, request-id, logging, recovery, auth
|   |-- server/                 http.Server, routes, middleware chain
|   +-- storage/                Postgres repositories and migrations
|-- migrations/                 001_init.sql, 002_bookmarks.sql, 003_api_keys.sql
|-- scripts/                    backup.sh (pg_dump + rotation)
|-- docs/                       this file, deployment, postmortem
|-- Dockerfile                  multi-stage build
|-- docker-compose.yml          development
|-- docker-compose.prod.yml     production
+-- Makefile
```

## 11. Technical Decisions

| Decision | Choice | Reason |
|---|---|---|
| Language | Go 1.27 | Static binary, first-class HTTP, small containers |
| HTTP layer | `net/http` + `ServeMux` | No framework to learn; Go 1.22 patterns cover 95% of routing needs |
| Database | Postgres 17 | Mature, JSON support, `gen_random_uuid()` built in |
| DB driver | `pgx/v5/stdlib` | Actively maintained, best Postgres support in Go |
| Migrations | Versioned `.sql`, embedded | Zero runtime dependency, auditable in git |
| Config | Environment variables | Works with Docker Compose and any orchestrator |
| Logging | `log/slog` | Structured, standard library |
| Auth | JWT (user) + API Keys (machine) | Stateless access for users, long-lived access for integrations |

## 12. Event Publishing (Fire-and-Forget)

When a bookmark is created, the API publishes a `bookmark.created` event to the Webhook Dispatcher.

This is a **fire-and-forget** operation:
- The event is pushed to a bounded buffered channel (capacity 256).
- Two background goroutines consume the channel and send HTTP POST requests to the Dispatcher.
- The API request returns `201 Created` immediately. It never blocks waiting for the Dispatcher.
- If the Dispatcher is unreachable, the error is logged in the background and the event is dropped. The user experience is unaffected.
```