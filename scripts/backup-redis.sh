#!/bin/bash
# Backup Redis database

# Load environment variables
if [ -f ../.env ]; then
    export $(grep -v '^#' ../.env | xargs)
fi

# Default values
REDIS_HOST=${REDIS_URL:-redis://localhost:6379}
BACKUP_DIR="${BACKUP_DIR:-../data/backups}"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/redis_backup_${TIMESTAMP}.rdb"

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Extract host and port from REDIS_URL
if [[ $REDIS_URL == redis://* ]]; then
    REDIS_HOST_PORT=$(echo $REDIS_URL | sed -e 's,redis\(s\)\?://,,' -e 's,/.*,,')
    REDIS_HOST=$(echo $REDIS_HOST_PORT | cut -d: -f1)
    REDIS_PORT=$(echo $REDIS_HOST_PORT | cut -d: -f2 -s)
    REDIS_PORT=${REDIS_PORT:-6379}
else
    REDIS_HOST="localhost"
    REDIS_PORT=6379
fi

echo "Backing up Redis database from ${REDIS_HOST}:${REDIS_PORT}..."

# Check if redis-cli is available
if ! command -v redis-cli &> /dev/null; then
    echo "Error: redis-cli is required but not installed." >&2
    exit 1
fi

# Create a backup
if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --rdb "$BACKUP_FILE"; then
    echo "Backup created successfully: $BACKUP_FILE"
    
    # Keep only the last 7 backups
    ls -t "$BACKUP_DIR"/redis_backup_*.rdb | tail -n +8 | xargs rm -f --
    
    echo "Kept only the 7 most recent backups."
else
    echo "Error: Failed to create Redis backup" >&2
    exit 1
fi
