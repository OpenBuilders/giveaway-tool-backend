# Миграция на PostgreSQL + Redis

Этот документ описывает процесс миграции с Redis-only архитектуры на гибридную архитектуру PostgreSQL + Redis.

## Архитектура

- **PostgreSQL**: Основное хранилище данных (пользователи, гивы, призы, участники, победы)
- **Redis**: Кэширование и временные данные (сессии, кэш запросов, очереди)

## Установка и настройка

### 1. PostgreSQL

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install postgresql postgresql-contrib

# macOS
brew install postgresql

# Создание базы данных
sudo -u postgres createdb giveaway_tool
sudo -u postgres createuser --interactive
```

### 2. Redis (для кэширования)

```bash
# Ubuntu/Debian
sudo apt install redis-server

# macOS
brew install redis
```

### 3. Переменные окружения

Создайте файл `.env`:

```env
# Server Configuration
SERVER_PORT=8080
SERVER_ORIGIN=http://localhost:3000

# PostgreSQL Configuration
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_password
POSTGRES_DB=giveaway_tool
POSTGRES_SSLMODE=disable
POSTGRES_MAX_OPEN_CONNS=25
POSTGRES_MAX_IDLE_CONNS=5
POSTGRES_CONN_MAX_LIFETIME=5m

# Redis Configuration (for caching)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_POOL_SIZE=10

# Telegram Configuration
BOT_TOKEN=your_bot_token_here
TELEGRAM_DEBUG=false
ADMIN_IDS=123456789,987654321

# Application Configuration
DEBUG=false
```

### 4. Миграция базы данных

```bash
# Подключитесь к PostgreSQL
psql -U postgres -d giveaway_tool

# Выполните миграцию
\i migrations/001_initial_schema.sql
```

## Структура базы данных

### Основные таблицы

- **users**: Пользователи системы
- **giveaways**: Гивы
- **participants**: Участники гивов
- **prizes**: Призы
- **giveaway_prizes**: Связь гивов и призов
- **win_records**: Записи о победах
- **requirements**: Требования для участия
- **sponsors**: Спонсоры (каналы)
- **giveaway_sponsors**: Связь гивов и спонсоров
- **tickets**: Билеты участников
- **pre_winner_lists**: Списки предварительных победителей

### Индексы

Созданы индексы для оптимизации запросов:
- По creator_id для быстрого поиска гивов пользователя
- По status для фильтрации по статусу
- По ends_at для поиска активных гивов
- Составные индексы для сложных запросов

## Преимущества новой архитектуры

### PostgreSQL
- **ACID транзакции**: Гарантии целостности данных
- **Сложные запросы**: JOIN, агрегации, подзапросы
- **Схема данных**: Строгая типизация и валидация
- **Индексы**: Оптимизация запросов
- **Резервное копирование**: Надежное восстановление данных

### Redis (кэширование)
- **Быстрый доступ**: Кэширование часто запрашиваемых данных
- **Сессии**: Временные данные пользователей
- **Очереди**: Фоновые задачи
- **Rate limiting**: Ограничение запросов

## API Endpoints

### Health Check
```
GET /health
```
Проверяет состояние PostgreSQL и Redis.

## Мониторинг

### PostgreSQL
```sql
-- Активные соединения
SELECT count(*) FROM pg_stat_activity WHERE state = 'active';

-- Медленные запросы
SELECT query, mean_time, calls 
FROM pg_stat_statements 
ORDER BY mean_time DESC 
LIMIT 10;
```

### Redis
```bash
# Статистика
redis-cli info

# Мониторинг команд
redis-cli monitor
```

## Миграция данных

Если у вас есть существующие данные в Redis, создайте скрипт миграции:

```go
// Пример миграции данных из Redis в PostgreSQL
func migrateFromRedis(redisClient *redis.Client, postgresDB *sql.DB) error {
    // Миграция пользователей
    // Миграция гивов
    // Миграция участников
    // и т.д.
}
```

## Производительность

### Оптимизация PostgreSQL
- Настройка shared_buffers
- Оптимизация work_mem
- Регулярная VACUUM
- Анализ статистики запросов

### Оптимизация Redis
- Настройка maxmemory
- Выбор правильной политики eviction
- Мониторинг hit/miss ratio

## Безопасность

### PostgreSQL
- Использование SSL соединений
- Ограничение доступа по IP
- Регулярное обновление паролей

### Redis
- Настройка аутентификации
- Ограничение доступа по IP
- Отключение опасных команд

## Резервное копирование

### PostgreSQL
```bash
# Создание бэкапа
pg_dump -U postgres giveaway_tool > backup.sql

# Восстановление
psql -U postgres giveaway_tool < backup.sql
```

### Redis
```bash
# Создание бэкапа
redis-cli BGSAVE

# Восстановление
redis-cli --pipe < dump.rdb
```

## Troubleshooting

### Частые проблемы

1. **Ошибка подключения к PostgreSQL**
   - Проверьте настройки в .env
   - Убедитесь, что PostgreSQL запущен
   - Проверьте права доступа пользователя

2. **Медленные запросы**
   - Анализируйте план выполнения запросов
   - Добавьте недостающие индексы
   - Оптимизируйте запросы

3. **Проблемы с Redis**
   - Проверьте доступность Redis
   - Мониторьте использование памяти
   - Проверьте настройки TTL

## Заключение

Гибридная архитектура PostgreSQL + Redis обеспечивает:
- Надежность и целостность данных
- Высокую производительность
- Масштабируемость
- Простоту разработки и поддержки 