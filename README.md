# giveaway-backend

Production-ready Go backend skeleton using net/http, Postgres (goose migrations), and Redis cache.

## Quick start

1. Copy env:
   ```bash
   cp .env.example .env
   ```
2. Install tools:
   ```bash
   go install github.com/pressly/goose/v3/cmd/goose@latest
   ```
3. Tidy deps:
   ```bash
   make tidy
   ```
4. Run migrations:
   ```bash
   make goose-up
   ```
5. Run server:
   ```bash
   make run
   ```

## Structure

- `cmd/api` - application entrypoint (HTTP server)
- `internal/config` - configuration loader
- `internal/domain/user` - domain model for user
- `internal/repository/postgres` - Postgres repositories
- `internal/cache/redis` - Redis cache implementations
- `internal/service` - business logic services
- `internal/http` - HTTP handlers (net/http)
- `internal/platform/db` - DB setup
- `internal/platform/redis` - Redis setup
- `migrations` - goose migrations

## Conventions
- English-only comments and docs
- Clean architecture-ish layering: handler -> service -> repo/cache
- net/http with stdlib only; add middleware minimally
- Context propagation everywhere
- Unit tests per package
