# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application (static binary)
RUN CGO_ENABLED=0 GOOS=linux go build -o /go/bin/ai-news-processor ./cmd/

# Final stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Set timezone
ENV TZ=Europe/Istanbul

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /go/bin/ai-news-processor .

# Copy web static files
COPY web ./web

# Create data directories
RUN mkdir -p /app/data/feeds /app/data/processed

# Expose application port (will be set by PORT env var)
EXPOSE 8080

# Environment variables
ENV PORT=8080
ENV APP_ENV=production
ENV STORAGE_PATH=/app/data
ENV PROCESSED_PATH=/app/data/processed/
ENV FEED_SOURCE_PATH=/app/data/feeds/
ENV REDIS_URL=redis://localhost:6379/0
ENV LOG_LEVEL=info

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT}/api/v1/health || exit 1

# Command to run the application
CMD ["./ai-news-processor"]
