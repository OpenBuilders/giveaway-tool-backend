#!/bin/bash

# Скрипт для запуска миграций PostgreSQL

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Функция для вывода сообщений
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

# Проверка наличия переменных окружения
if [ -z "$POSTGRES_HOST" ]; then
    POSTGRES_HOST="localhost"
fi

if [ -z "$POSTGRES_PORT" ]; then
    POSTGRES_PORT="5432"
fi

if [ -z "$POSTGRES_USER" ]; then
    POSTGRES_USER="postgres"
fi

if [ -z "$POSTGRES_DB" ]; then
    POSTGRES_DB="giveaway_tool"
fi

# Проверка подключения к PostgreSQL
log "Проверка подключения к PostgreSQL..."
if ! pg_isready -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER"; then
    error "Не удается подключиться к PostgreSQL"
    exit 1
fi

log "Подключение к PostgreSQL успешно"

# Создание базы данных, если она не существует
log "Проверка существования базы данных $POSTGRES_DB..."
if ! psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -lqt | cut -d \| -f 1 | grep -qw "$POSTGRES_DB"; then
    log "Создание базы данных $POSTGRES_DB..."
    createdb -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" "$POSTGRES_DB"
    log "База данных $POSTGRES_DB создана"
else
    log "База данных $POSTGRES_DB уже существует"
fi

# Запуск миграций
log "Запуск миграций..."

# Находим все файлы миграций
MIGRATION_FILES=$(find migrations -name "*.sql" | sort)

if [ -z "$MIGRATION_FILES" ]; then
    warn "Файлы миграций не найдены в директории migrations/"
    exit 0
fi

# Выполняем каждую миграцию
for migration_file in $MIGRATION_FILES; do
    log "Выполнение миграции: $migration_file"
    
    if psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "$migration_file"; then
        log "Миграция $migration_file выполнена успешно"
    else
        error "Ошибка при выполнении миграции $migration_file"
        exit 1
    fi
done

log "Все миграции выполнены успешно!"

# Проверка структуры базы данных
log "Проверка структуры базы данных..."
psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\dt"

log "Миграция завершена успешно!" 