#!/bin/bash
# Check application health

# Load environment variables
if [ -f ../.env ]; then
    export $(grep -v '^#' ../.env | xargs)
fi

# Default values
PORT=${PORT:-8080}
HEALTH_ENDPOINT="http://localhost:${PORT}/api/v1/health"
MAX_RETRIES=5
RETRY_DELAY=2

# Check if curl is available
if ! command -v curl &> /dev/null; then
    echo "Error: curl is required but not installed." >&2
    exit 1
fi

# Function to check health
check_health() {
    local status_code
    status_code=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_ENDPOINT")
    
    if [ "$status_code" -eq 200 ]; then
        echo "Application is healthy (Status: $status_code)"
        return 0
    else
        echo "Health check failed (Status: $status_code)" >&2
        return 1
    fi
}

# Try multiple times with delay
for ((i=1; i<=MAX_RETRIES; i++)); do
    echo "Checking application health (Attempt $i/$MAX_RETRIES)..."
    if check_health; then
        exit 0
    fi
    
    if [ $i -lt $MAX_RETRIES ]; then
        echo "Retrying in $RETRY_DELAY seconds..."
        sleep $RETRY_DELAY
    fi
done

echo "Health check failed after $MAX_RETRIES attempts" >&2
exit 1
