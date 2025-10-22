#!/bin/bash
# Clean up old processed files and logs

# Load environment variables
if [ -f ../.env ]; then
    export $(grep -v '^#' ../.env | xargs)
fi

# Default values
DATA_DIR="${STORAGE_PATH:-../data}"
RETENTION_DAYS=${RETENTION_DAYS:-30}
LOG_FILE="${LOG_FILE:-../logs/cleanup.log}"

# Create log directory if it doesn't exist
mkdir -p "$(dirname "$LOG_FILE")"

# Function to log messages
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

log "Starting cleanup process..."

# Clean up old processed files
if [ -d "$DATA_DIR/processed" ]; then
    log "Removing processed files older than $RETENTION_DAYS days..."
    find "$DATA_DIR/processed" -type f -name "*.json" -mtime +$RETENTION_DAYS -exec rm -v {} \; | tee -a "$LOG_FILE"
    
    # Remove empty directories
    find "$DATA_DIR/processed" -type d -empty -delete
    log "Cleanup of processed files completed."
else
    log "Processed directory not found: $DATA_DIR/processed"
fi

# Clean up old log files
if [ -d "$(dirname "$LOG_FILE")" ]; then
    log "Removing log files older than 7 days..."
    find "$(dirname "$LOG_FILE")" -name "*.log" -mtime +7 -exec rm -v {} \; | tee -a "$LOG_FILE"
    log "Cleanup of log files completed."
fi

log "Cleanup process finished."
