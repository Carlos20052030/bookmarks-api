# Deployment — Bookmarks API

How to run this service in production on a Linux VPS with Docker Compose, integrated with the Nuntix ecosystem.

> Repository: https://github.com/Carlos20052030/bookmarks-api

## Overview

The production stack is part of a larger ecosystem (Nuntix). The containers involved are:

- **`api`** — the Go binary. It listens internally on port `8080`, but is mapped to the host as `127.0.0.1:8081` to avoid conflicts. It is not directly exposed to the internet.
- **`db`** — Postgres 17. It is mapped to `127.0.0.1:5432` so the Nuntix Proxy (running on the host network) and the Webhook Dispatcher can access it.
- **`reverse-proxy`** (Nuntix) — Sits in front, handles TLS (Let's Encrypt), and routes public traffic to the API.
- **`webhook-dispatcher`** — Connects to the same database to process webhook deliveries.

## Requirements

- A Linux VPS with a public IP (tested on Ubuntu 26.04, Hetzner CX23)
- SSH access with a non-root user that has `sudo`
- Docker Engine 29+ and the Compose plugin
- About 400 MB of free disk (image + database)

## 1. Install Docker

Skip if `docker --version` already works.

```bash
sudo apt update
sudo apt install -y ca-certificates curl gnupg

sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | \
    sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | \
    sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io \
    docker-buildx-plugin docker-compose-plugin

sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"
```

Log out and back in so the group change takes effect. Verify:

```bash
docker --version
docker compose version
```

## 2. Clone the repository

```bash
cd ~
git clone https://github.com/Carlos20052030/bookmarks-api.git
cd bookmarks-api
```

## 3. Configure

Copy the example file to create your local `.env`:

```bash
cp .env.example .env
```

Generate two secrets and paste them into the `.env` file:

```bash
openssl rand -base64 32   # POSTGRES_PASSWORD (must be URL-encoded if used in URLs)
openssl rand -base64 48   # JWT_SECRET
```

The `.env` file must contain:

```env
POSTGRES_USER=bookmarks
POSTGRES_PASSWORD=<generated>
POSTGRES_DB=bookmarks
POSTGRES_HOST=db
POSTGRES_PORT=5432
SERVER_PORT=8080
LOG_LEVEL=info
JWT_SECRET=<generated>

# Webhook Dispatcher integration
# Points to the dispatcher container on the shared Docker network
DISPATCHER_URL=http://webhook-dispatcher-dispatcher-1:8080
INGEST_TOKEN=<generated-must-match-dispatcher>
```

`.env` is gitignored. Never commit it.

## 4. Build and start

Before starting this, ensure the `webhook-dispatcher` network exists (it is created automatically when the dispatcher starts). Then:

```bash
docker compose up -d --build
```

The `api` container waits for the `db` healthcheck before starting.

## 5. Verify

```bash
docker ps
```

Check that `bookmarks-api-api-1` and `bookmarks-api-db-1` are `(healthy)`.

```bash
curl -s http://127.0.0.1:8081/health
# {"status":"ok"}
```

Test the full register flow from outside the VPS (using the Nuntix Proxy domain):

```bash
curl -s -X POST https://api.nuntix.com.br/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"smoke@test.com","password":"correcthorse"}'
```

Expected: `201 Created` with `user`, `access_token`, and `refresh_token`.

## 6. Backups

A script in `scripts/backup.sh` runs `pg_dump` inside the `db` container,
compresses the result, and keeps the last 4 copies.

Run manually once to confirm it works:

```bash
./scripts/backup.sh
ls -lh ~/backups/bookmarks/
```

Install the cron job:

```bash
crontab -e
```

Add this line (adjust paths if needed):

```
0 3 * * * cd /home/carlos/bookmarks-api && ./scripts/backup.sh >> /home/carlos/backups/bookmarks/backup.log 2>&1
```

### Restore procedure

Tested procedure to restore the latest backup into a scratch database:

```bash
# Create an empty database.
docker exec -it bookmarks-api-db-1 \
    psql -U bookmarks -d postgres -c "CREATE DATABASE restore_test;"

# Restore the backup into it.
gunzip -c ~/backups/bookmarks/bookmarks-<timestamp>.sql.gz | \
    docker exec -i bookmarks-api-db-1 \
    psql -U bookmarks -d restore_test

# Confirm data.
docker exec -it bookmarks-api-db-1 \
    psql -U bookmarks -d restore_test -c "SELECT COUNT(*) FROM users;"

# Drop the scratch database.
docker exec -it bookmarks-api-db-1 \
    psql -U bookmarks -d postgres -c "DROP DATABASE restore_test;"
```

**A backup that has never been restored is not a backup.** Run this once
after installing the service, and again whenever the schema changes.

## 7. Updating the service

```bash
cd ~/bookmarks-api
git pull origin main
docker compose up -d --build
```

Migrations run automatically on boot. To see logs:

```bash
docker logs -f bookmarks-api-api-1
```

## 8. Operations

| Task | Command |
|---|---|
| Status | `docker ps` |
| API logs | `docker logs -f bookmarks-api-api-1` |
| DB logs | `docker logs -f bookmarks-api-db-1` |
| Restart API only | `docker restart bookmarks-api-api-1` |
| Stop everything | `docker compose down` |
| Stop, **delete data** | `docker compose down -v` |

## 9. Troubleshooting

**`address already in use` on port 8081**

Something else is listening. Find and stop it:

```bash
sudo ss -tlnp | grep 8081
```

**`db` never becomes healthy**

Check the logs:

```bash
docker logs bookmarks-api-db-1 | tail -30
```

Common causes: wrong password (a stale volume holds the old one), disk full,
or Postgres failed to start.

**Stale password after editing `.env`**

Postgres stores the password on first init. Changing `.env` does not update
an existing volume. To reset:

```bash
docker compose down -v   # deletes data
docker compose up -d
```

## 10. Security Notes

- **Secrets live in `.env` and never in git.** The `.gitignore` blocks
  `.env` and the `.dockerignore` blocks it from the build context.
- **The database is only exposed to localhost.** The `docker-compose.yml` maps the database port to `127.0.0.1:5432`. This means only the host (and the Nuntix Proxy, which runs on the host network) can access it. It is not exposed to the public internet.
- **The API runs as a non-root user inside the container.** The Dockerfile
  creates a dedicated `app` user.
- **JWT_SECRET must be at least 32 bytes.** The config loader refuses to
  start with a shorter value.
- **Rotate secrets by editing `.env` and restarting the stack.** Existing
  JWTs signed with the old secret become invalid.
- **The `INGEST_TOKEN` must be identical to the Webhook Dispatcher's token.** This authenticates internal service-to-service communication.
- **CORS is restricted** to localhost (for development) and `https://nuntix.com.br` (for production).
```