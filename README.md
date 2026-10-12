# Bookmarks API

![Go Version](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-green)
![Status](https://img.shields.io/badge/status-live-brightgreen)

A REST API for managing private bookmarks with tags. JWT authentication,
Postgres persistence, strict per-user ownership, and no HTTP framework —
just the Go standard library.

## Features

- **JWT authentication** — short-lived access tokens (15 min) and opaque refresh tokens (7 days) with rotation
- **Refresh token reuse detection** — presenting a revoked token revokes
  every session for that user. Follows the principles for refresh token
  rotation in RFC 9700 (OAuth 2.0 Security Best Current Practice)
- **Timing attack mitigation** — runs a bcrypt verification against a
  dummy hash when the email is unknown, so the login path takes
  approximately the same time whether the account exists or not
- **Strict ownership** — every query is scoped by user ID; cross-user access returns `404`, not `403`
- **Bookmarks with tags** — full CRUD, pagination, tag filtering, attach/detach
- **Structured logging** — one log line per request with method, path, status, duration, and request ID
- **Graceful shutdown** — in-flight requests finish before the process exits
- **Rate limiting** — token bucket per IP on `/auth/login` (10/min) and `/auth/register` (5/min); returns `429` with `Retry-After`
- **Page title fetch** — fetches `<title>` on bookmark creation when no title is given, with SSRF protection (private, loopback, and link-local addresses are refused)
- **Zero framework** — `net/http` with Go 1.22+ `ServeMux` patterns; `database/sql` with the `pgx` driver

## Tech Stack

| Layer | Choice |
|---|---|
| Language | Go 1.27 |
| HTTP | `net/http` (stdlib) |
| Database | Postgres 17 |
| DB driver | `github.com/jackc/pgx/v5/stdlib` |
| Auth | JWT HS256 + opaque refresh tokens |
| JWT library | `github.com/golang-jwt/jwt/v5` |
| Password hashing | `golang.org/x/crypto/bcrypt` (cost 12) |
| Token hashing | `crypto/sha256` (stdlib) |
| Migrations | Versioned `.sql` files, embedded with `//go:embed` |
| Container | Docker (multi-stage, `alpine` runtime) |

## Quick Start

### Prerequisites

- Go 1.27+
- Docker and Docker Compose

### 1. Clone

```bash
git clone https://github.com/Carlos20052030/bookmarks-api.git
cd bookmarks-api
```

### 2. Configure

```bash
cp .env.example .env
```

Generate secrets and paste them into `.env`:

```bash
openssl rand -base64 32   # POSTGRES_PASSWORD
openssl rand -base64 48   # JWT_SECRET
```

### 3. Start Postgres

```bash
docker compose up -d db
```

### 4. Run the API

```bash
make run
```

The server listens on `:8080`. Migrations run automatically on boot.

### 5. Verify

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

## API Endpoints

### Public

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness probe |
| `POST` | `/auth/register` | Create account, returns tokens |
| `POST` | `/auth/login` | Exchange credentials for tokens |
| `POST` | `/auth/refresh` | Rotate refresh token, get a new pair |
| `POST` | `/auth/logout` | Revoke refresh token |

### Protected

Require `Authorization: Bearer <access_token>`.

| Method | Path | Description |
|---|---|---|
| `POST` | `/bookmarks` | Create bookmark |
| `GET` | `/bookmarks` | List with pagination and `?tag=<uuid>` filter |
| `GET` | `/bookmarks/{id}` | Get one with tags |
| `PUT` | `/bookmarks/{id}` | Update |
| `DELETE` | `/bookmarks/{id}` | Delete |
| `POST` | `/bookmarks/{id}/tags/{tagID}` | Attach tag |
| `DELETE` | `/bookmarks/{id}/tags/{tagID}` | Detach tag |
| `POST` | `/tags` | Create tag |
| `GET` | `/tags` | List tags |
| `DELETE` | `/tags/{id}` | Delete tag |

## Example: full auth flow

### 1. Register

```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"correcthorse"}'
```

```json
{
  "user": {
    "id": "9aa3e19a-ad7f-4fe7-b072-ac351dc98e5d",
    "email": "user@example.com",
    "created_at": "2026-10-12T10:00:00Z"
  },
  "access_token": "eyJhbGciOi...",
  "refresh_token": "X5F_8nVz...",
  "token_type": "Bearer",
  "expires_in": 900
}
```

### 2. Create a bookmark

```bash
TOKEN="eyJhbGciOi..."

curl -s -X POST http://localhost:8080/bookmarks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/doc/","title":"Go Documentation"}'
```

```json
{
  "id": "41132bc5-91f2-451c-b8d5-ded39a5ce6c9",
  "url": "https://go.dev/doc/",
  "title": "Go Documentation",
  "tags": [],
  "created_at": "2026-10-12T10:00:05Z",
  "updated_at": "2026-10-12T10:00:05Z"
}
```

### 3. Refresh

```bash
curl -s -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"X5F_8nVz..."}'
```

```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "_7rLYtDg...",
  "token_type": "Bearer",
  "expires_in": 900
}
```

## Project Structure

```
bookmarks-api/
├── cmd/api/                  Application entry point
├── internal/
│   ├── auth/                 JWT, bcrypt, refresh token primitives
│   ├── config/               Environment variable loading and validation
│   ├── domain/               Core types (User, Bookmark, Tag, RefreshToken)
│   ├── handler/              HTTP handlers
│   ├── middleware/           Request ID, logging, recovery, auth
│   ├── server/               HTTP server wiring
│   └── storage/              Postgres repositories and migrations
├── migrations/               Versioned SQL files, embedded in the binary
├── Dockerfile                Multi-stage production build
├── docker-compose.yml        Development stack
├── docker-compose.prod.yml   Production stack
└── Makefile
```

## Design Notes

- **UUIDs everywhere.** Sequential IDs leak information — a user can guess how many records exist, or probe `/bookmarks/1`, `/bookmarks/2`. UUIDv4 makes enumeration infeasible.
- **`404`, not `403`, for another user's resource.** Returning `403` confirms the resource exists. `404` is indistinguishable from "not found", which leaks nothing.
- **SHA-256 for tokens, bcrypt for passwords.** Tokens generated by `crypto/rand` carry 256 bits of entropy — bcrypt only adds latency without meaningfully increasing difficulty. Passwords are low-entropy and need the work factor.
- **Refresh token rotation.** Every refresh revokes the old token and issues a new one. A reused token signals theft and revokes every session for that user (OAuth 2.0 refresh token reuse detection).
- **Migrations in transactions.** Each migration runs in its own transaction and is recorded in `schema_migrations`. Re-running the service is a no-op.

## Deployment

See [`docs/deployment.md`](docs/deployment.md) for the production setup (Docker Compose on a VPS).

## License

MIT — see [LICENSE](LICENSE).
