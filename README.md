# Giveaway Tool Backend

Backend service for managing giveaways and user participations.

## Features

- User management (registration, authentication, profile management)
- Giveaway management (creation, updating, deletion)
- Participation management (joining, leaving giveaways)
- Winner selection
- Statistics and analytics

## Technology Stack

- Go 1.21
- Gin Web Framework
- PostgreSQL
- Redis
- JWT Authentication
- Docker & Docker Compose

## Project Structure

```
.
├── cmd/                    # Application entry points
│   └── app/               # Main application
├── internal/              # Private application code
│   ├── common/           # Shared code
│   │   ├── auth/        # Authentication
│   │   ├── config/      # Configuration
│   │   ├── errors/      # Error handling
│   │   ├── logger/      # Logging
│   │   ├── middleware/  # HTTP middleware
│   │   └── validator/   # Validation
│   ├── features/        # Feature modules
│   │   ├── giveaway/    # Giveaway feature
│   │   ├── participation/ # Participation feature
│   │   └── user/        # User feature
│   └── platform/        # Platform-specific code
│       ├── database/    # Database
│       └── redis/       # Redis
├── scripts/             # Scripts
├── config/             # Configuration files
├── .env.example        # Environment variables example
├── docker-compose.yml  # Docker Compose configuration
├── Dockerfile         # Docker configuration
└── README.md         # Project documentation
```

## Getting Started

### Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Make (optional)

### Local Development

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/giveaway-tool-backend.git
   cd giveaway-tool-backend
   ```

2. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

3. Start the development environment:
   ```bash
   docker-compose up -d
   ```

4. Run database migrations:
   ```bash
   ./scripts/migrate.sh up
   ```

5. Start the application:
   ```bash
   go run cmd/app/main.go
   ```

### API Documentation

The API documentation is available at `/swagger/index.html` when running the application.

## API Endpoints

### Authentication
- POST `/api/v1/auth/register` - Register a new user
- POST `/api/v1/auth/login` - Login user

### Users
- GET `/api/v1/users/:id` - Get user by ID
- PUT `/api/v1/users/:id` - Update user
- DELETE `/api/v1/users/:id` - Delete user
- GET `/api/v1/users` - List users
- POST `/api/v1/users/:id/change-password` - Change password
- PUT `/api/v1/users/:id/status` - Update user status

### Giveaways
- POST `/api/v1/giveaways` - Create giveaway
- GET `/api/v1/giveaways/:id` - Get giveaway by ID
- PUT `/api/v1/giveaways/:id` - Update giveaway
- DELETE `/api/v1/giveaways/:id` - Delete giveaway
- GET `/api/v1/giveaways` - List giveaways
- GET `/api/v1/giveaways/user/:userId` - List user's giveaways
- POST `/api/v1/giveaways/:id/select-winner` - Select winner

### Participations
- POST `/api/v1/participations/join` - Join giveaway
- POST `/api/v1/participations/:id/leave` - Leave giveaway
- GET `/api/v1/participations/:id` - Get participation by ID
- GET `/api/v1/participations/giveaway/:giveawayId` - List giveaway participations
- GET `/api/v1/participations/user/:userId` - List user participations
- GET `/api/v1/participations/giveaway/:giveawayId/stats` - Get giveaway stats
- GET `/api/v1/participations/check/:giveawayId` - Check participation status

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Переменные окружения

Для работы приложения необходимо установить следующие переменные окружения:

```bash
# Режим отладки
DEBUG=false

# Настройки сервера
PORT=8080

# Настройки Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# Настройки Telegram
BOT_TOKEN=your-bot-token-here
TELEGRAM_DEBUG=false
ADMIN_IDS=123456789,987654321  # Список ID администраторов через запятую
```

Вы можете создать файл `.env` в корне проекта или установить переменные окружения напрямую в системе. 