package errors

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// ErrorCode представляет код ошибки
type ErrorCode string

const (
	// Общие ошибки
	ErrCodeInternal        ErrorCode = "INTERNAL_ERROR"
	ErrCodeValidation      ErrorCode = "VALIDATION_ERROR"
	ErrCodeNotFound        ErrorCode = "NOT_FOUND"
	ErrCodeUnauthorized    ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden       ErrorCode = "FORBIDDEN"
	ErrCodeConflict        ErrorCode = "CONFLICT"
	ErrCodeTooManyRequests ErrorCode = "TOO_MANY_REQUESTS"
	ErrCodeBadRequest      ErrorCode = "BAD_REQUEST"

	// Ошибки пользователей
	ErrCodeUserNotFound    ErrorCode = "USER_NOT_FOUND"
	ErrCodeUserBanned      ErrorCode = "USER_BANNED"
	ErrCodeUserInactive    ErrorCode = "USER_INACTIVE"
	ErrCodeInvalidUserData ErrorCode = "INVALID_USER_DATA"

	// Ошибки гивов
	ErrCodeGiveawayNotFound ErrorCode = "GIVEAWAY_NOT_FOUND"
	ErrCodeGiveawayExpired  ErrorCode = "GIVEAWAY_EXPIRED"
	ErrCodeGiveawayFull     ErrorCode = "GIVEAWAY_FULL"
	ErrCodeAlreadyJoined    ErrorCode = "ALREADY_JOINED"
	ErrCodeNotOwner         ErrorCode = "NOT_OWNER"
	ErrCodeInvalidWinners   ErrorCode = "INVALID_WINNERS_COUNT"

	// Ошибки каналов
	ErrCodeChannelNotFound ErrorCode = "CHANNEL_NOT_FOUND"
	ErrCodeChannelInvalid  ErrorCode = "CHANNEL_INVALID"

	// Ошибки базы данных
	ErrCodeDatabaseError     ErrorCode = "DATABASE_ERROR"
	ErrCodeTransactionFailed ErrorCode = "TRANSACTION_FAILED"
	ErrCodeConnectionFailed  ErrorCode = "CONNECTION_FAILED"

	// Ошибки кэша
	ErrCodeCacheError ErrorCode = "CACHE_ERROR"
	ErrCodeCacheMiss  ErrorCode = "CACHE_MISS"

	// Ошибки внешних API
	ErrCodeTelegramAPI ErrorCode = "TELEGRAM_API_ERROR"
	ErrCodeExternalAPI ErrorCode = "EXTERNAL_API_ERROR"
	ErrCodeRateLimit   ErrorCode = "RATE_LIMIT_EXCEEDED"
)

// AppError представляет типизированную ошибку приложения
type AppError struct {
	Code      ErrorCode              `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Context   map[string]string      `json:"context,omitempty"`
	Stack     []string               `json:"stack,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	RequestID string                 `json:"request_id,omitempty"`
	UserID    int64                  `json:"user_id,omitempty"`
	Cause     error                  `json:"-"`
}

// Error возвращает строковое представление ошибки
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap возвращает причину ошибки
func (e *AppError) Unwrap() error {
	return e.Cause
}

// IsNotFound проверяет, является ли ошибка ошибкой "не найдено"
func (e *AppError) IsNotFound() bool {
	return e.Code == ErrCodeNotFound ||
		e.Code == ErrCodeUserNotFound ||
		e.Code == ErrCodeGiveawayNotFound ||
		e.Code == ErrCodeChannelNotFound
}

// IsValidation проверяет, является ли ошибка ошибкой валидации
func (e *AppError) IsValidation() bool {
	return e.Code == ErrCodeValidation || e.Code == ErrCodeInvalidUserData
}

// IsUnauthorized проверяет, является ли ошибка ошибкой авторизации
func (e *AppError) IsUnauthorized() bool {
	return e.Code == ErrCodeUnauthorized || e.Code == ErrCodeForbidden
}

// IsInternal проверяет, является ли ошибка внутренней ошибкой
func (e *AppError) IsInternal() bool {
	return e.Code == ErrCodeInternal ||
		e.Code == ErrCodeDatabaseError ||
		e.Code == ErrCodeCacheError ||
		e.Code == ErrCodeTelegramAPI
}

// WithContext добавляет контекст к ошибке
func (e *AppError) WithContext(key, value string) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]string)
	}
	e.Context[key] = value
	return e
}

// WithDetail добавляет детальную информацию к ошибке
func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WithRequestID добавляет ID запроса к ошибке
func (e *AppError) WithRequestID(requestID string) *AppError {
	e.RequestID = requestID
	return e
}

// WithUserID добавляет ID пользователя к ошибке
func (e *AppError) WithUserID(userID int64) *AppError {
	e.UserID = userID
	return e
}

// WithStack добавляет стек вызовов к ошибке
func (e *AppError) WithStack() *AppError {
	e.Stack = getStackTrace()
	return e
}

// New создает новую ошибку приложения
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
		Stack:     getStackTrace(),
	}
}

// Wrap оборачивает существующую ошибку
func Wrap(err error, code ErrorCode, message string) *AppError {
	appErr := New(code, message)
	appErr.Cause = err
	return appErr
}

// Wrapf оборачивает существующую ошибку с форматированием
func Wrapf(err error, code ErrorCode, format string, args ...interface{}) *AppError {
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

// getStackTrace возвращает стек вызовов
func getStackTrace() []string {
	var stack []string
	for i := 2; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}
		// Пропускаем внутренние функции пакета errors
		if strings.Contains(fn.Name(), "internal/common/errors") {
			continue
		}
		stack = append(stack, fmt.Sprintf("%s:%d %s", file, line, fn.Name()))
		if len(stack) >= 10 { // Ограничиваем глубину стека
			break
		}
	}
	return stack
}

// Конструкторы для часто используемых ошибок

// NewValidationError создает ошибку валидации
func NewValidationError(field, reason string) *AppError {
	return New(ErrCodeValidation, fmt.Sprintf("Validation failed for field '%s': %s", field, reason)).
		WithDetail("field", field).
		WithDetail("reason", reason)
}

// NewNotFoundError создает ошибку "не найдено"
func NewNotFoundError(resource, id interface{}) *AppError {
	return New(ErrCodeNotFound, fmt.Sprintf("%s not found", resource)).
		WithDetail("resource", resource).
		WithDetail("id", id)
}

// NewUserNotFoundError создает ошибку "пользователь не найден"
func NewUserNotFoundError(userID int64) *AppError {
	return New(ErrCodeUserNotFound, fmt.Sprintf("User not found: %d", userID)).
		WithDetail("user_id", userID)
}

// NewGiveawayNotFoundError создает ошибку "гив не найден"
func NewGiveawayNotFoundError(giveawayID string) *AppError {
	return New(ErrCodeGiveawayNotFound, fmt.Sprintf("Giveaway not found: %s", giveawayID)).
		WithDetail("giveaway_id", giveawayID)
}

// NewChannelNotFoundError создает ошибку "канал не найден"
func NewChannelNotFoundError(channelID int64) *AppError {
	return New(ErrCodeChannelNotFound, fmt.Sprintf("Channel not found: %d", channelID)).
		WithDetail("channel_id", channelID)
}

// NewUnauthorizedError создает ошибку авторизации
func NewUnauthorizedError(reason string) *AppError {
	return New(ErrCodeUnauthorized, fmt.Sprintf("Unauthorized: %s", reason)).
		WithDetail("reason", reason)
}

// NewForbiddenError создает ошибку доступа
func NewForbiddenError(reason string) *AppError {
	return New(ErrCodeForbidden, fmt.Sprintf("Forbidden: %s", reason)).
		WithDetail("reason", reason)
}

// NewDatabaseError создает ошибку базы данных
func NewDatabaseError(operation string, err error) *AppError {
	return Wrap(err, ErrCodeDatabaseError, fmt.Sprintf("Database operation failed: %s", operation)).
		WithDetail("operation", operation)
}

// NewCacheError создает ошибку кэша
func NewCacheError(operation string, err error) *AppError {
	return Wrap(err, ErrCodeCacheError, fmt.Sprintf("Cache operation failed: %s", operation)).
		WithDetail("operation", operation)
}

// NewTelegramAPIError создает ошибку Telegram API
func NewTelegramAPIError(operation string, err error) *AppError {
	return Wrap(err, ErrCodeTelegramAPI, fmt.Sprintf("Telegram API operation failed: %s", operation)).
		WithDetail("operation", operation)
}

// NewRateLimitError создает ошибку превышения лимита запросов
func NewRateLimitError(service string, retryAfter time.Duration) *AppError {
	return New(ErrCodeRateLimit, fmt.Sprintf("Rate limit exceeded for %s", service)).
		WithDetail("service", service).
		WithDetail("retry_after", retryAfter.String())
}

// NewConflictError создает ошибку конфликта
func NewConflictError(resource, reason string) *AppError {
	return New(ErrCodeConflict, fmt.Sprintf("Conflict with %s: %s", resource, reason)).
		WithDetail("resource", resource).
		WithDetail("reason", reason)
}

// IsAppError проверяет, является ли ошибка AppError
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// AsAppError приводит ошибку к AppError
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if err != nil {
		appErr, _ = err.(*AppError)
	}
	return appErr, appErr != nil
}
