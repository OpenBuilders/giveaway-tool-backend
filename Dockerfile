# Build stage
FROM golang:1.23.9-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/app

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/main .
COPY config/config.yaml ./config/

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Set environment variables
ENV GIN_MODE=release

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"] 