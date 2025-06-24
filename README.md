# Giveaway Tool Backend

Backend для Telegram Mini App гивов с гибридной архитектурой PostgreSQL + Redis.

## Архитектура

### База данных
- **PostgreSQL** - основное хранилище данных (гивы, пользователи, каналы, призы, участники)
- **Redis** - кэширование часто запрашиваемых данных

### Основные компоненты

#### Features (Модули)
- **giveaway** - управление гивами, призами, участниками
- **user** - управление пользователями
- **channel** - управление каналами и спонсорами
- **tonproof** - TON Proof интеграция

#### Platform (Инфраструктура)
- **postgres** - PostgreSQL клиент и миграции
- **redis** - Redis клиент для кэширования
- **telegram** - Telegram Bot API клиент

#### Common (Общие компоненты)
- **cache** - кэш-сервис для Redis
- **config** - конфигурация приложения
- **logger** - логирование
- **middleware** - HTTP middleware

## Установка и запуск

### Требования
- Go 1.21+
- PostgreSQL 14+
- Redis 6+

### Настройка окружения
```bash
cp env.example .env
# Отредактируйте .env файл
```

### Запуск с Docker
```bash
# Запуск всех сервисов
make docker-run

# Просмотр логов
make docker-logs

# Остановка
make docker-stop
```

### Запуск для разработки
```bash
# Запуск только БД
make dev

# Или вручную
docker-compose up -d postgres redis
./scripts/migrate.sh
go run cmd/app/main.go
```

### Миграции
```bash
# Применение миграций
make migrate

# Создание новой миграции
./scripts/migrate.sh create migration_name
```

## API Endpoints

### Пользователи
- `GET /api/v1/users/me` - информация о текущем пользователе
- `GET /api/v1/users/me/stats` - статистика пользователя
- `GET /api/v1/users/me/giveaways` - гивы пользователя
- `GET /api/v1/users/me/wins` - победы пользователя

### Гивы
- `POST /api/v1/giveaways` - создание гива
- `GET /api/v1/giveaways` - список гивов
- `GET /api/v1/giveaways/:id` - информация о гиве
- `POST /api/v1/giveaways/:id/join` - участие в гиве
- `GET /api/v1/giveaways/:id/winners` - победители гива

### Каналы
- `GET /api/v1/channels/:id` - информация о канале
- `GET /api/v1/channels/username/:username` - информация по username

## Кэширование

### Стратегия кэширования
- **Пользователи**: 30 минут
- **Статистика пользователей**: 15 минут
- **Гивы пользователей**: 10 минут
- **Каналы**: 30 минут
- **Аватары каналов**: 30 минут

### Инвалидация кэша
Кэш автоматически инвалидируется при:
- Обновлении данных пользователя
- Изменении гива
- Обновлении информации о канале

## Разработка

### Структура проекта
```
internal/
├── features/           # Бизнес-логика
│   ├── giveaway/      # Гивы
│   ├── user/          # Пользователи
│   ├── channel/       # Каналы
│   └── tonproof/      # TON Proof
├── platform/          # Инфраструктура
│   ├── postgres/      # PostgreSQL
│   ├── redis/         # Redis
│   └── telegram/      # Telegram API
└── common/            # Общие компоненты
    ├── cache/         # Кэширование
    ├── config/        # Конфигурация
    ├── logger/        # Логирование
    └── middleware/    # HTTP middleware
```

### Добавление нового модуля
1. Создайте структуру в `internal/features/`
2. Добавьте модели в `models/`
3. Создайте репозиторий в `repository/postgres/`
4. Добавьте сервис в `service/`
5. Создайте HTTP handler в `delivery/http/`
6. Добавьте кэширование в сервис

### Тестирование
```bash
# Запуск тестов
make test

# Запуск тестов с покрытием
go test -cover ./...
```

## Мониторинг

### Health Check
- `GET /health` - проверка состояния сервисов

### Логирование
- Структурированные логи в JSON формате
- Уровни: debug, info, warn, error, fatal

## Безопасность

### Аутентификация
- Telegram Mini App init_data
- Валидация подписи Telegram

### Валидация данных
- Входные данные валидируются через binding
- SQL injection защита через prepared statements
- XSS защита через middleware

## Производительность

### Оптимизации
- Кэширование часто запрашиваемых данных
- Индексы в PostgreSQL
- Connection pooling
- Gzip сжатие ответов

### Мониторинг
- Метрики PostgreSQL
- Метрики Redis
- HTTP метрики

## Развертывание

### Docker
```bash
# Сборка образа
make docker-build

# Запуск
make docker-run
```

### Kubernetes
```bash
# Применение манифестов
kubectl apply -f deployments/
```

## Лицензия

Apache 2.0
