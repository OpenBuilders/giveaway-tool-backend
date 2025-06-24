.PHONY: build run test clean migrate docker-build docker-run docker-stop docker-logs dev clean-all help

# Переменные
BINARY_NAME=giveaway-tool-backend
DOCKER_IMAGE=giveaway-tool-backend
DOCKER_TAG=latest

# Сборка приложения
build:
	go build -o bin/$(BINARY_NAME) cmd/app/main.go

# Запуск приложения
run:
	go run cmd/app/main.go

# Тестирование
test:
	go test -v ./...

# Очистка
clean:
	rm -f bin/$(BINARY_NAME)
	go clean

# Миграции базы данных
migrate:
	./scripts/migrate.sh

# Сборка Docker образа
docker-build:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Запуск в Docker
docker-run:
	docker-compose up -d

# Остановка Docker
docker-stop:
	docker-compose down

# Просмотр логов
docker-logs:
	docker-compose logs -f

# Разработка
dev:
	docker-compose up -d postgres redis
	sleep 5
	./scripts/migrate.sh
	go run cmd/app/main.go

# Полная очистка
clean-all: clean docker-stop
	docker system prune -f
	docker volume prune -f

# Помощь
help:
	@echo "Доступные команды:"
	@echo "  build                    - Сборка приложения"
	@echo "  run                      - Запуск приложения"
	@echo "  test                     - Запуск тестов"
	@echo "  clean                    - Очистка бинарных файлов"
	@echo "  migrate                  - Запуск миграций PostgreSQL"
	@echo "  docker-build             - Сборка Docker образа"
	@echo "  docker-run               - Запуск в Docker"
	@echo "  docker-stop              - Остановка Docker"
	@echo "  docker-logs              - Просмотр логов"
	@echo "  dev                      - Запуск для разработки"
	@echo "  clean-all                - Полная очистка"
	@echo "  help                     - Показать эту справку" 