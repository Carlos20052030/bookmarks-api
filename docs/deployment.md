# Deployment — Bookmarks API

How to run this service in production on a Linux VPS with Docker Compose.

> Repository: https://github.com/Carlos20052030/bookmarks-api

## Overview

The production stack has two containers:

- **`api`** — the Go binary, exposed on port `8080`
- **`db`** — Postgres 17, reachable only from the `api` container over the
  Compose network

Postgres is **not** exposed to the host. The only open port is `8080`.

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

```bash
cp .env.example .env
```

Generate two secrets and paste them into `.env`:

```bash
openssl rand -base64 32   # POSTGRES_PASSWORD
openssl rand -base64 48   # JWT_SECRET
```

The `.env` file must contain:

```
POSTGRES_USER=bookmarks
POSTGRES_PASSWORD=<generated>
POSTGRES_DB=bookmarks
POSTGRES_HOST=db
POSTGRES_PORT=5432
SERVER_PORT=8080
LOG_LEVEL=info
JWT_SECRET=<generated>
```

`.env` is gitignored. Never commit it.

## 4. Build and start

```bash
docker compose -f docker-compose.prod.yml build
docker compose -f docker-compose.prod.yml up -d
```

The `api` container waits for the `db` healthcheck before starting.

## 5. Verify

```bash
docker compose -f docker-compose.prod.yml ps
```

Both containers should show `(healthy)`.

```bash
curl -s http://localhost:8080/health
# {"status":"ok"}
```

From outside the VPS:

```bash
curl -s http://<vps-ip>:8080/health
```

Full register flow:

```bash
curl -s -X POST http://<vps-ip>:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"smoke@test.com","password":"correcthorse"}'
```

Expected: `201 Created` with `user`, `access_token`, and `refresh_token`.

## 6. Backups

A script in `scripts/backup.sh` runs `pg_dump` inside the `db` container,
compresses the result, and keeps the last 4 copies.

Run manually once to confirm it works:

```bash
COMPOSE_FILE=docker-compose.prod.yml ./scripts/backup.sh
ls -lh ~/backups/bookmarks/
```

Install the cron job:

```bash
crontab -e
```

Add this line (adjust paths if needed):

```
0 3 * * * cd /home/carlos/bookmarks-api && COMPOSE_FILE=docker-compose.prod.yml ./scripts/backup.sh >> /home/carlos/backups/bookmarks/backup.log 2>&1
```

### Restore procedure

Tested procedure to restore the latest backup into a scratch database:

```bash
# Create an empty database.
docker compose -f docker-compose.prod.yml exec db \
    psql -U bookmarks -d postgres -c "CREATE DATABASE restore_test;"

# Restore the backup into it.
gunzip -c ~/backups/bookmarks/bookmarks-<timestamp>.sql.gz | \
    docker compose -f docker-compose.prod.yml exec -T db \
    psql -U bookmarks -d restore_test

# Confirm data.
docker compose -f docker-compose.prod.yml exec db \
    psql -U bookmarks -d restore_test -c "SELECT COUNT(*) FROM users;"

# Drop the scratch database.
docker compose -f docker-compose.prod.yml exec db \
    psql -U bookmarks -d postgres -c "DROP DATABASE restore_test;"
```

**A backup that has never been restored is not a backup.** Run this once
after installing the service, and again whenever the schema changes.

## 7. Updating the service

```bash
cd ~/bookmarks-api
git pull
docker compose -f docker-compose.prod.yml build
docker compose -f docker-compose.prod.yml up -d
```

Migrations run automatically on boot. To see logs:

```bash
docker compose -f docker-compose.prod.yml logs -f api
```

## 8. Operations

| Task | Command |
|---|---|
| Status | `docker compose -f docker-compose.prod.yml ps` |
| API logs | `docker compose -f docker-compose.prod.yml logs -f api` |
| DB logs | `docker compose -f docker-compose.prod.yml logs -f db` |
| Restart API only | `docker compose -f docker-compose.prod.yml restart api` |
| Stop everything | `docker compose -f docker-compose.prod.yml down` |
| Stop, keep data | `docker compose -f docker-compose.prod.yml down` (no `-v`) |
| Stop, **delete data** | `docker compose -f docker-compose.prod.yml down -v` |

## 9. Troubleshooting

**`address already in use` on port 8080**

Something else is listening. Find and stop it:

```bash
sudo ss -tlnp | grep 8080
```

**`db` never becomes healthy**

Check the logs:

```bash
docker compose -f docker-compose.prod.yml logs db | tail -30
```

Common causes: wrong password (a stale volume holds the old one), disk full,
or Postgres failed to start.

**Stale password after editing `.env`**

Postgres stores the password on first init. Changing `.env` does not update
an existing volume. To reset:

```bash
docker compose -f docker-compose.prod.yml down -v   # deletes data
docker compose -f docker-compose.prod.yml up -d
```

**Firewall appears open on ports you never allowed**

Docker inserts its own iptables rules that bypass `ufw`. The `ufw status`
output is not a complete view of what is exposed. Ports published with
`ports:` in the compose file are reachable from outside regardless of
`ufw` rules.

## 10. Security Notes

- **Secrets live in `.env` and never in git.** The `.gitignore` blocks
  `.env` and the `.dockerignore` blocks it from the build context.
- **Postgres has no exposed port.** No `ports:` entry for the `db` service
  in `docker-compose.prod.yml`. Only `expose: 5432`.
- **The API runs as a non-root user inside the container.** The Dockerfile
  creates a dedicated `app` user.
- **JWT_SECRET must be at least 32 bytes.** The config loader refuses to
  start with a shorter value.
- **Rotate secrets by editing `.env` and restarting the stack.** Existing
  JWTs signed with the old secret become invalid.