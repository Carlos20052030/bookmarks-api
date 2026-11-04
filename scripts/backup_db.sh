#!/usr/bin/env bash
#
# Automated PostgreSQL database backup for bookmarks-api
# Creates compressed SQL dumps with timestamp, retains 7 days locally
#

set -euo pipefail

# Determine project root (parent of this script's directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "${SCRIPT_DIR}")"

# Source environment variables from .env file
if [[ -f "${PROJECT_ROOT}/.env" ]]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
else
    echo "ERROR: .env file not found at ${PROJECT_ROOT}/.env"
    exit 1
fi

# Configuration (using environment variables with defaults)
BACKUP_DIR="/home/carlos/backups"
CONTAINER_NAME="bookmarks-api-db-1"
DB_USER="${POSTGRES_USER:-postgres}"
DB_NAME="${POSTGRES_DB:-bookmarks}"
TIMESTAMP=$(date +"%Y-%m-%d_%H-%M")
BACKUP_FILE="${BACKUP_DIR}/nuntix_backup_${TIMESTAMP}.sql.gz"

# Create backup directory if it doesn't exist
mkdir -p "${BACKUP_DIR}"

# Perform backup using docker exec and pg_dump
echo "Starting backup at $(date)"
echo "Using database user: ${DB_USER}, database: ${DB_NAME}"
docker exec "${CONTAINER_NAME}" pg_dump -U "${DB_USER}" -d "${DB_NAME}" --clean --if-exists | gzip > "${BACKUP_FILE}"

# Verify backup was created and has content
if [[ -f "${BACKUP_FILE}" && -s "${BACKUP_FILE}" ]]; then
    SIZE=$(stat -c%s "${BACKUP_FILE}")
    echo "Backup completed successfully: ${BACKUP_FILE} (${SIZE} bytes)"
else
    echo "ERROR: Backup failed or file is empty"
    rm -f "${BACKUP_FILE}"
    exit 1
fi

# Clean up backups older than 7 days
echo "Cleaning up backups older than 7 days..."
find "${BACKUP_DIR}" -type f -name "*.sql.gz" -mtime +7 -delete
echo "Cleanup completed"

echo "Backup process finished at $(date)"