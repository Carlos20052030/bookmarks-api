#!/usr/bin/env bash
#
# Backup the bookmarks-api Postgres database.
#
# Runs pg_dump inside the db container, compresses the output,
# and keeps only the N most recent backups.
#
# Usage:
#   ./scripts/backup.sh
#   COMPOSE_FILE=docker-compose.prod.yml ./scripts/backup.sh
#
set -euo pipefail

# --- configuration ----------------------------------------------------------

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
BACKUP_DIR="${BACKUP_DIR:-$HOME/backups/bookmarks}"
RETENTION="${RETENTION:-4}"
DB_SERVICE="db"
DB_USER="${POSTGRES_USER:-bookmarks}"
DB_NAME="${POSTGRES_DB:-bookmarks}"

# --- paths ------------------------------------------------------------------

TIMESTAMP="$(date +%Y-%m-%d-%H%M)"
BACKUP_FILE="$BACKUP_DIR/bookmarks-$TIMESTAMP.sql.gz"

# --- helpers ----------------------------------------------------------------

log() {
    printf '[%s] %s\n' "$(date -Iseconds)" "$*"
}

die() {
    log "ERROR: $*" >&2
    exit 1
}

# --- preconditions ----------------------------------------------------------

command -v docker >/dev/null 2>&1 || die "docker is not installed"
[ -f "$COMPOSE_FILE" ] || die "compose file not found: $COMPOSE_FILE"

mkdir -p "$BACKUP_DIR"
[ -w "$BACKUP_DIR" ] || die "backup directory is not writable: $BACKUP_DIR"

# --- dump -------------------------------------------------------------------

log "starting backup of $DB_NAME -> $BACKUP_FILE"

if ! docker compose -f "$COMPOSE_FILE" exec -T "$DB_SERVICE" \
        pg_dump -U "$DB_USER" -d "$DB_NAME" --clean --if-exists \
        | gzip > "$BACKUP_FILE"; then
    rm -f "$BACKUP_FILE"
    die "pg_dump failed"
fi

# A 1 KB gzip is almost certainly an empty or corrupt dump.
SIZE=$(stat -c %s "$BACKUP_FILE")
if [ "$SIZE" -lt 1024 ]; then
    rm -f "$BACKUP_FILE"
    die "backup file is suspiciously small ($SIZE bytes)"
fi

log "backup complete: $BACKUP_FILE ($SIZE bytes)"

# --- retention --------------------------------------------------------------

cd "$BACKUP_DIR"
mapfile -t OLD < <(ls -1t bookmarks-*.sql.gz 2>/dev/null | tail -n +$((RETENTION + 1)))
for f in "${OLD[@]}"; do
    rm -f "$f"
    log "removed old backup: $f"
done

log "done"