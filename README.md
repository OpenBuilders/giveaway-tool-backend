# 🎁 Giveaway Tool – Backend

## Summary

Giveaway Tool is a comprehensive backend service for managing and conducting giveaways within Telegram communities. It provides robust infrastructure for creating, managing, and executing fair and transparent giveaways with blockchain integration and advanced requirement verification.

* Giveaway bot: [@giveaway_app_bot](https://t.me/giveaway_app_bot)

### Key Features

* **Giveaway Management**: Create and manage giveaways with multiple prizes and custom requirements
* **TON Blockchain Integration**: Verify wallet ownership through TON Proof and check on-chain balances
* **Telegram Integration**: Full integration with Telegram API for channel management and user verification
* **Flexible Requirements System**: Support for custom requirements including:
  * Channel subscriptions
  * Premium Telegram status
  * TON wallet balance checks
  * On-chain asset verification
* **Participant Management**: Track participants and verify their eligibility in real-time
* **Winner Selection**: Fair and random winner selection with prize distribution
* **Notifications System**: Automated notifications for participants and winners
* **Caching Layer**: Redis-based caching for optimal performance
* **RESTful API**: Clean and well-structured API endpoints

### Technology Stack

* **Backend**: Go 1.23 with Fiber framework
* **Database**: PostgreSQL 16 with Goose migrations
* **Caching & Streams**: Redis 7
* **Blockchain**: TON integration with Tongo library
* **Authentication**: Telegram Mini Apps Init Data validation
* **Containerization**: Docker and Docker Compose

## Installation

To install and run the project, follow these steps:

1. **Prerequisites**:
   * Docker and Docker Compose must be installed on your machine
   * Go 1.23 or higher (for local development)
   * Goose migration tool for database management
   * A Telegram Bot Token (obtain from [@BotFather](https://t.me/BotFather))

2. **Setup**:
   * Clone the repository:
     ```bash
     git clone https://github.com/OpenBuilders/giveaway-tool-backend.git
     cd giveaway-tool-backend
     ```
   
   * Create environment configuration:
     ```bash
     cp .env.example .env
     ```
     Edit `.env` with your configuration (see Configuration section)
   
   * Install Goose for migrations:
     ```bash
     go install github.com/pressly/goose/v3/cmd/goose@latest
     ```
   
   * Install dependencies:
     ```bash
     make tidy
     ```

3. **Running with Docker (Recommended)**:
   * Build and start all services:
     ```bash
     docker-compose up -d
     ```
   * The API will be accessible at `http://localhost:8081`

4. **Running locally (Development)**:
   * Ensure PostgreSQL and Redis are running
   * Run migrations:
     ```bash
     make goose-up
     ```
   * Start the API:
     ```bash
     make run
     ```

## Configuration

The application uses environment variables for configuration. Create a `.env` file in the root directory with the following variables:

```env
# HTTP Server
HTTP_ADDR=:8080

# Database
DATABASE_URL=postgres://user:password@localhost:5432/giveaway?sslmode=disable
POSTGRES_USER=user
POSTGRES_PASSWORD=password
POSTGRES_DB=giveaway

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# Telegram
TELEGRAM_BOT_TOKEN=your_bot_token_here

# Security
INIT_DATA_TTL=86400  # 24 hours in seconds

# CORS
CORS_ALLOWED_ORIGINS=*
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `HTTP_ADDR` | HTTP server address | `:8080` |
| `DATABASE_URL` | PostgreSQL connection string | - |
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `REDIS_PASSWORD` | Redis password (if required) | - |
| `REDIS_DB` | Redis database number | `0` |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token from @BotFather | - |
| `INIT_DATA_TTL` | Init data validation TTL in seconds | `86400` |
| `CORS_ALLOWED_ORIGINS` | Allowed CORS origins | `*` |

## Usage

### Available Commands

The project includes several make commands to simplify development and operation:

* `make tidy`: Tidy Go module dependencies
* `make build`: Build the application binary
* `make run`: Run the application locally
* `make test`: Run all tests
* `make lint`: Run golangci-lint for code quality
* `make goose-up`: Run database migrations
* `make goose-down`: Rollback last migration
* `make goose-status`: Check migration status
* `make migrate-create name=<migration_name>`: Create a new migration

### Docker Commands

* Start all services:
  ```bash
  docker-compose up -d
  ```

* Stop all services:
  ```bash
  docker-compose down
  ```

* View logs:
  ```bash
  docker-compose logs -f api
  ```

* Rebuild containers:
  ```bash
  docker-compose up -d --build
  ```

### Project Structure

The project follows clean architecture principles with clear separation of concerns:

* **`cmd/api`**: Application entrypoint and HTTP server initialization
* **`internal/config`**: Configuration loading and validation
* **`internal/domain`**: Domain models and interfaces
  * `giveaway`: Giveaway domain models and repository interfaces
  * `user`: User domain models and repository interfaces
* **`internal/repository/postgres`**: PostgreSQL repository implementations
* **`internal/cache/redis`**: Redis cache implementations
  * `user_cache.go`: User data caching
  * `channel_avatar_cache.go`: Channel avatar URL caching
  * `channel_photo_cache.go`: Channel photo caching
* **`internal/service`**: Business logic layer
  * `giveaway`: Giveaway management service
  * `user`: User management service
  * `telegram`: Telegram API client wrapper
  * `tonproof`: TON Proof verification service
  * `tonbalance`: TON balance checking service
  * `channels`: Telegram channel management
  * `notifications`: Notification dispatch service
* **`internal/http`**: HTTP handlers and routing
  * `fiber_app.go`: Fiber application setup
  * `giveaway_handlers.go`: Giveaway endpoints
  * `user_handlers.go`: User endpoints
  * `requirements_handlers.go`: Requirements verification endpoints
  * `tonproof_handlers.go`: TON Proof endpoints
  * `channel_handlers.go`: Channel endpoints
  * `middleware/`: HTTP middleware (init data validation, caching)
* **`internal/platform`**: Infrastructure setup
  * `db`: PostgreSQL connection
  * `redis`: Redis connection
* **`internal/workers`**: Background workers
  * `redis_stream.go`: Redis stream consumer for async tasks
* **`internal/utils`**: Utility functions
  * `random`: Random number generation and shuffling
  * `telegram`: Telegram-specific utilities
* **`migrations`**: Database migrations (Goose format)

### API Endpoints

The API provides the following endpoint groups:

* **Giveaways**: Create, update, retrieve, and manage giveaways
* **Participants**: Register participants and check eligibility
* **Winners**: Select winners and retrieve winner lists
* **Requirements**: Verify participant requirements
* **Users**: User profile management
* **Channels**: Channel information and verification
* **TON Proof**: Wallet verification and proof generation

## Contributing

We welcome contributions to Giveaway Tool! Here's how you can contribute:

### Development Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/your-feature-name`
3. Set up your local environment (see Installation section)
4. Make your changes
5. Run tests: `make test`
6. Run linter: `make lint`
7. Commit your changes: `git commit -m "feat: add some feature"`
8. Push to the branch: `git push origin feat/your-feature-name`
9. Submit a pull request

### Coding Standards

* Go code should follow standard Go conventions and pass `golangci-lint`
* Use meaningful variable and function names
* Add comments for exported functions and types
* Keep functions small and focused on a single responsibility
* Write tests for new features and bug fixes
* Ensure all tests pass before submitting PR

### Commit Message Convention

We follow conventional commits format:

* `feat:` - New feature
* `fix:` - Bug fix
* `docs:` - Documentation changes
* `refactor:` - Code refactoring
* `test:` - Adding or updating tests
* `chore:` - Maintenance tasks

### Testing

Before submitting a pull request, ensure that:

1. All tests pass:
   ```bash
   make test
   ```

2. Code passes linting:
   ```bash
   make lint
   ```

3. Migrations work correctly:
   ```bash
   make goose-up
   make goose-down
   ```

## Architecture

The application follows Clean Architecture principles:

```
┌─────────────────────────────────────────────────────┐
│                   HTTP Layer                        │
│              (Fiber Handlers)                       │
└────────────────────┬────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────┐
│                 Service Layer                       │
│           (Business Logic)                          │
└────────────────────┬────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────┬──────────────────────┐
│    Repository Layer          │    Cache Layer       │
│    (PostgreSQL)              │    (Redis)           │
└──────────────────────────────┴──────────────────────┘
```

### Design Principles

* **Dependency Injection**: Services receive dependencies through constructors
* **Interface-based Design**: Domain layer defines interfaces, implementations are in infrastructure layer
* **Context Propagation**: Context is passed through all layers for cancellation and tracing
* **Error Handling**: Consistent error handling with proper error wrapping
* **Caching Strategy**: Strategic caching of frequently accessed data (user info, channel data)

## License

This project is licensed under the MIT License — see the LICENSE file for details.

## Acknowledgements

* Giveaway Tool is developed and maintained by Open Builders
* Built with ❤️ for the Telegram and TON communities
* Special thanks to all contributors who have helped shape this project

## Support

For issues, questions, or contributions, please:
* Open an issue on GitHub
* Submit a pull request
* Try the bot: [@giveaway_app_bot](https://t.me/giveaway_app_bot)
* Contact the maintainers

---

**🤖 Bot**: [@giveaway_app_bot](https://t.me/giveaway_app_bot)

Built by [Open Builders](https://github.com/OpenBuilders) | Part of the [Tools.tg](https://tools.tg) ecosystem
