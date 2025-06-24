# Giveaway Tool Backend

## Architecture

### Data Storage
- **PostgreSQL** - primary data store (giveaways, users, channels, prizes, participants)
- **Redis** - caching layer for frequently accessed data

### Core Components

#### Features (Business Logic)
- **giveaway** - giveaway management, prizes, participants
- **user** - user management and authentication
- **channel** - channel and sponsor management
- **tonproof** - TON Proof integration

#### Platform (Infrastructure)
- **postgres** - PostgreSQL client and migrations
- **redis** - Redis client for caching
- **telegram** - Telegram Bot API client

#### Common (Shared Components)
- **cache** - Redis cache service
- **config** - application configuration
- **errors** - typed error handling
- **middleware** - HTTP middleware
- **validation** - input validation

## Quick Start

### Prerequisites
- Go 1.21+
- PostgreSQL 14+
- Redis 6+

### Setup
```bash
# Clone and setup
git clone <repository>
cd giveaway-tool-backend

# Configure environment
cp env.example .env
# Edit .env with your settings

# Run with Docker
make docker-run

# Or run locally
make dev
go run cmd/app/main.go
```

### Development
```bash
# Start dependencies only
docker-compose up -d postgres redis

# Run migrations
make migrate

# Start application
go run cmd/app/main.go
```

## API Overview

### Authentication
All endpoints require Telegram Mini App `init_data` for authentication.

### Key Endpoints
- `POST /api/v1/giveaways` - Create giveaway
- `GET /api/v1/giveaways/:id` - Get giveaway details
- `POST /api/v1/giveaways/:id/join` - Join giveaway
- `GET /api/v1/users/me` - Current user info
- `GET /api/v1/channels/me` - User's channels

## Project Structure

```
internal/
├── features/           # Business logic modules
│   ├── giveaway/      # Giveaway management
│   ├── user/          # User management
│   ├── channel/       # Channel management
│   └── tonproof/      # TON Proof integration
├── platform/          # Infrastructure
│   ├── postgres/      # PostgreSQL client
│   ├── redis/         # Redis client
│   └── telegram/      # Telegram API client
└── common/            # Shared components
    ├── cache/         # Caching service
    ├── config/        # Configuration
    ├── errors/        # Error handling
    ├── middleware/    # HTTP middleware
    └── validation/    # Input validation
```

## Development Guide

### Adding a New Feature
1. Create module structure in `internal/features/`
2. Add models in `models/`
3. Create repository in `repository/postgres/`
4. Add service in `service/`
5. Create HTTP handler in `delivery/http/`
6. Add caching to service

### Code Style
- Use structured logging with zap
- Implement proper error handling with typed errors
- Add input validation for all endpoints
- Use transactions for data consistency
- Implement caching for frequently accessed data

### Testing
```bash
# Run tests
make test

# Run with coverage
go test -cover ./...
```

## Caching Strategy

### Cache TTL
- **Users**: 30 minutes
- **User stats**: 15 minutes
- **User giveaways**: 10 minutes
- **Channels**: 30 minutes
- **Channel avatars**: 30 minutes

### Cache Invalidation
Cache is automatically invalidated when:
- User data is updated
- Giveaway is modified
- Channel information changes

## Error Handling

The project uses a comprehensive error handling system:
- Typed errors with context
- Proper HTTP status codes
- Structured error responses
- Error logging with stack traces

## Security

- Telegram Mini App authentication
- Input validation and sanitization
- SQL injection protection
- Rate limiting
- CORS configuration

## Performance

- Redis caching for hot data
- PostgreSQL connection pooling
- Optimized database queries
- Gzip compression
- Health checks and monitoring

## Deployment

### Docker
```bash
make docker-build
make docker-run
```

### Environment Variables
See `env.example` for required environment variables.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

Apache 2.0
