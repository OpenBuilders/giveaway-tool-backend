#!/bin/bash

# Check if migrate tool is installed
if ! command -v migrate &> /dev/null; then
    echo "migrate tool is not installed. Installing..."
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
fi

# Load environment variables from .env file
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Default values
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-postgres}
DB_NAME=${DB_NAME:-giveaway}
DB_SSL_MODE=${DB_SSL_MODE:-disable}

# Build connection string
DB_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSL_MODE}"

# Check command line arguments
ACTION=$1
if [ -z "$ACTION" ]; then
    echo "Please specify an action: up or down"
    exit 1
fi

# Run migrations
echo "Running migrations ${ACTION}..."
migrate -database "${DB_URL}" -path internal/platform/database/migrations "$ACTION"

# Check migration status
if [ $? -eq 0 ]; then
    echo "Migration ${ACTION} completed successfully"
else
    echo "Migration ${ACTION} failed"
    exit 1
fi 