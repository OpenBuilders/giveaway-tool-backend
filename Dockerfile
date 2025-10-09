# syntax=docker/dockerfile:1

## Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app

# Install build deps
RUN apk add --no-cache git build-base

# Go env for faster builds
ENV CGO_ENABLED=0 \
    GO111MODULE=on

# Cache modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the API binary
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/app ./cmd/api

## Runtime stage
FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=builder /out/app /app

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app"]


