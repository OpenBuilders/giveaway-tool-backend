# Система обработки ошибок

## Обзор

Система обработки ошибок в Giveaway Tool Backend предоставляет типизированные ошибки с контекстом, трейсингом и детальной информацией для эффективного мониторинга и отладки.

## Основные компоненты

### 1. Типизированные ошибки (`internal/common/errors/errors.go`)

Система использует структуру `AppError` для представления всех ошибок приложения:

```go
type AppError struct {
    Code      ErrorCode            `json:"code"`
    Message   string               `json:"message"`
    Details   map[string]interface{} `json:"details,omitempty"`
    Context   map[string]string    `json:"context,omitempty"`
    Stack     []string             `json:"stack,omitempty"`
    Timestamp time.Time            `json:"timestamp"`
    RequestID string               `json:"request_id,omitempty"`
    UserID    int64                `json:"user_id,omitempty"`
    Cause     error                `json:"-"`
}
```

### 2. Коды ошибок

Система поддерживает следующие типы ошибок:

#### Общие ошибки
- `INTERNAL_ERROR` - Внутренняя ошибка сервера
- `VALIDATION_ERROR` - Ошибка валидации
- `NOT_FOUND` - Ресурс не найден
- `UNAUTHORIZED` - Не авторизован
- `FORBIDDEN` - Доступ запрещен
- `CONFLICT` - Конфликт ресурсов
- `TOO_MANY_REQUESTS` - Превышен лимит запросов
- `BAD_REQUEST` - Неверный запрос

#### Ошибки пользователей
- `USER_NOT_FOUND` - Пользователь не найден
- `USER_BANNED` - Пользователь заблокирован
- `USER_INACTIVE` - Пользователь неактивен
- `INVALID_USER_DATA` - Неверные данные пользователя

#### Ошибки гивов
- `GIVEAWAY_NOT_FOUND` - Гив не найден
- `GIVEAWAY_EXPIRED` - Гив истек
- `GIVEAWAY_FULL` - Гив заполнен
- `ALREADY_JOINED` - Уже участвует
- `NOT_OWNER` - Не владелец
- `INVALID_WINNERS_COUNT` - Неверное количество победителей

#### Ошибки каналов
- `CHANNEL_NOT_FOUND` - Канал не найден
- `CHANNEL_INVALID` - Неверный канал

#### Ошибки базы данных
- `DATABASE_ERROR` - Ошибка базы данных
- `TRANSACTION_FAILED` - Ошибка транзакции
- `CONNECTION_FAILED` - Ошибка подключения

#### Ошибки кэша
- `CACHE_ERROR` - Ошибка кэша
- `CACHE_MISS` - Кэш не найден

#### Ошибки внешних API
- `TELEGRAM_API_ERROR` - Ошибка Telegram API
- `EXTERNAL_API_ERROR` - Ошибка внешнего API
- `RATE_LIMIT_EXCEEDED` - Превышен лимит запросов

## Использование

### Создание ошибок

```go
import "giveaway-tool-backend/internal/common/errors"

// Простая ошибка
err := errors.New(errors.ErrCodeValidation, "Invalid input data")

// Ошибка с деталями
err := errors.NewValidationError("username", "Username must be at least 5 characters").
    WithDetail("provided_value", username).
    WithDetail("min_length", 5)

// Ошибка с контекстом
err := errors.NewDatabaseError("create user", dbErr).
    WithContext("table", "users").
    WithContext("operation", "INSERT")

// Ошибка с трейсингом
err := errors.New(errors.ErrCodeInternal, "Processing failed").
    WithStack()
```

### Конструкторы для часто используемых ошибок

```go
// Ошибки валидации
err := errors.NewValidationError("field", "reason")

// Ошибки "не найдено"
err := errors.NewUserNotFoundError(userID)
err := errors.NewGiveawayNotFoundError(giveawayID)
err := errors.NewChannelNotFoundError(channelID)

// Ошибки авторизации
err := errors.NewUnauthorizedError("Invalid token")
err := errors.NewForbiddenError("Insufficient permissions")

// Ошибки базы данных
err := errors.NewDatabaseError("operation", originalError)

// Ошибки кэша
err := errors.NewCacheError("operation", originalError)

// Ошибки Telegram API
err := errors.NewTelegramAPIError("operation", originalError)

// Ошибки превышения лимита
err := errors.NewRateLimitError("service", retryAfter)

// Ошибки конфликта
err := errors.NewConflictError("resource", "reason")
```

### Проверка типов ошибок

```go
if appErr, ok := errors.AsAppError(err); ok {
    switch {
    case appErr.IsValidation():
        // Обработка ошибки валидации
    case appErr.IsNotFound():
        // Обработка ошибки "не найдено"
    case appErr.IsUnauthorized():
        // Обработка ошибки авторизации
    case appErr.IsInternal():
        // Обработка внутренней ошибки
    }
}
```

### Middleware для обработки ошибок

```go
import "giveaway-tool-backend/internal/common/middleware"

// Добавляем middleware в Gin
router.Use(middleware.RequestID())
router.Use(middleware.ErrorHandler(logger))
router.Use(gin.Recovery())
```

## Логирование

Система автоматически логирует ошибки с контекстом:

```go
logger.Error("Application error occurred",
    zap.String("request_id", requestID),
    zap.String("error_code", string(appErr.Code)),
    zap.String("error_message", appErr.Message),
    zap.Time("timestamp", appErr.Timestamp),
    zap.Any("details", appErr.Details),
    zap.Any("context", appErr.Context),
    zap.Strings("stack", appErr.Stack),
)
```

## HTTP ответы

Система автоматически преобразует ошибки в соответствующие HTTP статус коды:

- `VALIDATION_ERROR` → 400 Bad Request
- `NOT_FOUND` → 404 Not Found
- `UNAUTHORIZED` → 401 Unauthorized
- `FORBIDDEN` → 403 Forbidden
- `CONFLICT` → 409 Conflict
- `TOO_MANY_REQUESTS` → 429 Too Many Requests
- `DATABASE_ERROR` → 500 Internal Server Error
- `CACHE_ERROR` → 503 Service Unavailable

## Пример ответа API

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed for field 'username': Username must be at least 5 characters",
    "details": {
      "field": "username",
      "reason": "Username must be at least 5 characters",
      "provided_value": "john",
      "min_length": 5
    },
    "context": {
      "operation": "user_creation",
      "endpoint": "/api/v1/users"
    },
    "stack": [
      "internal/features/user/service/service.go:45 validateUserData",
      "internal/features/user/service/service.go:23 CreateUser"
    ],
    "timestamp": "2024-03-15T14:30:00Z",
    "request_id": "req-12345"
  },
  "timestamp": "2024-03-15T14:30:00Z",
  "request_id": "req-12345",
  "path": "/api/v1/users",
  "method": "POST"
}
```

## Лучшие практики

### 1. Всегда используйте типизированные ошибки

```go
// ❌ Плохо
return fmt.Errorf("user not found: %d", userID)

// ✅ Хорошо
return errors.NewUserNotFoundError(userID).
    WithDetail("requested_id", userID)
```

### 2. Добавляйте контекст к ошибкам

```go
err := errors.NewDatabaseError("get user", dbErr).
    WithContext("table", "users").
    WithContext("operation", "SELECT").
    WithDetail("user_id", userID)
```

### 3. Используйте детали для отладки

```go
err := errors.NewValidationError("email", "Invalid email format").
    WithDetail("provided_value", email).
    WithDetail("expected_format", "user@domain.com")
```

### 4. Добавляйте трейсинг для критических ошибок

```go
err := errors.New(errors.ErrCodeInternal, "Critical processing error").
    WithStack().
    WithDetail("processing_step", "prize_distribution")
```

### 5. Проверяйте типы ошибок

```go
if appErr, ok := errors.AsAppError(err); ok {
    if appErr.IsValidation() {
        // Возвращаем 400 Bad Request
        return
    }
    if appErr.IsNotFound() {
        // Возвращаем 404 Not Found
        return
    }
}
```

## Мониторинг и алерты

Система позволяет легко настроить мониторинг на основе кодов ошибок:

```go
// Пример метрики для Prometheus
if appErr.IsInternal() {
    internalErrorsCounter.Inc()
}

if appErr.IsValidation() {
    validationErrorsCounter.Inc()
}
```

## Отладка

Для отладки используйте детали и стек ошибок:

```go
if appErr, ok := errors.AsAppError(err); ok {
    fmt.Printf("Error Code: %s\n", appErr.Code)
    fmt.Printf("Message: %s\n", appErr.Message)
    fmt.Printf("Details: %+v\n", appErr.Details)
    fmt.Printf("Context: %+v\n", appErr.Context)
    fmt.Printf("Stack: %s\n", strings.Join(appErr.Stack, "\n"))
}
``` 