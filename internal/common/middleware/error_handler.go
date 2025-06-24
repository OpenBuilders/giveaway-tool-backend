package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"giveaway-tool-backend/internal/common/errors"
)

// ErrorHandler middleware для обработки ошибок
func ErrorHandler(logger *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		requestID := getRequestID(c)

		// Логируем панику
		logger.Error("Panic recovered",
			zap.String("request_id", requestID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Any("panic", recovered),
			zap.String("stack", string(debug.Stack())),
		)

		// Создаем ошибку паники
		appErr := errors.New(errors.ErrCodeInternal, "Internal server error").
			WithRequestID(requestID).
			WithDetail("panic", fmt.Sprintf("%v", recovered)).
			WithDetail("stack", string(debug.Stack()))

		// Отправляем ответ
		sendErrorResponse(c, appErr, logger)
	})
}

// RequestID middleware для добавления ID запроса
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// ErrorResponse представляет ответ с ошибкой
type ErrorResponse struct {
	Success   bool             `json:"success"`
	Error     *errors.AppError `json:"error"`
	Timestamp time.Time        `json:"timestamp"`
	RequestID string           `json:"request_id"`
	Path      string           `json:"path,omitempty"`
	Method    string           `json:"method,omitempty"`
}

// sendErrorResponse отправляет ошибку в формате JSON
func sendErrorResponse(c *gin.Context, appErr *errors.AppError, logger *zap.Logger) {
	requestID := getRequestID(c)

	// Добавляем контекст запроса к ошибке
	appErr.WithRequestID(requestID).
		WithContext("path", c.Request.URL.Path).
		WithContext("method", c.Request.Method)

	// Определяем HTTP статус код
	statusCode := getHTTPStatusCode(appErr)

	// Создаем ответ
	response := ErrorResponse{
		Success:   false,
		Error:     appErr,
		Timestamp: time.Now(),
		RequestID: requestID,
		Path:      c.Request.URL.Path,
		Method:    c.Request.Method,
	}

	// Логируем ошибку
	logError(appErr, logger, c)

	// Отправляем ответ
	c.JSON(statusCode, response)
}

// getHTTPStatusCode возвращает HTTP статус код для ошибки
func getHTTPStatusCode(appErr *errors.AppError) int {
	switch appErr.Code {
	case errors.ErrCodeValidation, errors.ErrCodeInvalidUserData, errors.ErrCodeBadRequest:
		return http.StatusBadRequest
	case errors.ErrCodeNotFound, errors.ErrCodeUserNotFound, errors.ErrCodeGiveawayNotFound, errors.ErrCodeChannelNotFound:
		return http.StatusNotFound
	case errors.ErrCodeUnauthorized:
		return http.StatusUnauthorized
	case errors.ErrCodeForbidden, errors.ErrCodeNotOwner:
		return http.StatusForbidden
	case errors.ErrCodeConflict, errors.ErrCodeAlreadyJoined:
		return http.StatusConflict
	case errors.ErrCodeTooManyRequests, errors.ErrCodeRateLimit:
		return http.StatusTooManyRequests
	case errors.ErrCodeGiveawayExpired, errors.ErrCodeGiveawayFull:
		return http.StatusGone
	case errors.ErrCodeUserBanned, errors.ErrCodeUserInactive:
		return http.StatusForbidden
	case errors.ErrCodeDatabaseError, errors.ErrCodeTransactionFailed, errors.ErrCodeConnectionFailed:
		return http.StatusInternalServerError
	case errors.ErrCodeCacheError, errors.ErrCodeCacheMiss:
		return http.StatusServiceUnavailable
	case errors.ErrCodeTelegramAPI, errors.ErrCodeExternalAPI:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// logError логирует ошибку с контекстом
func logError(appErr *errors.AppError, logger *zap.Logger, c *gin.Context) {
	requestID := getRequestID(c)
	userID := getUserID(c)

	// Создаем поля для логирования
	fields := []zap.Field{
		zap.String("request_id", requestID),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.String("error_code", string(appErr.Code)),
		zap.String("error_message", appErr.Message),
		zap.Time("timestamp", appErr.Timestamp),
	}

	if userID != 0 {
		fields = append(fields, zap.Int64("user_id", userID))
	}

	if appErr.UserID != 0 {
		fields = append(fields, zap.Int64("error_user_id", appErr.UserID))
	}

	if len(appErr.Details) > 0 {
		detailsJSON, _ := json.Marshal(appErr.Details)
		fields = append(fields, zap.String("details", string(detailsJSON)))
	}

	if len(appErr.Context) > 0 {
		contextJSON, _ := json.Marshal(appErr.Context)
		fields = append(fields, zap.String("context", string(contextJSON)))
	}

	if appErr.Cause != nil {
		fields = append(fields, zap.Error(appErr.Cause))
	}

	// Выбираем уровень логирования
	switch {
	case appErr.IsInternal():
		logger.Error("Internal error occurred", fields...)
	case appErr.IsUnauthorized():
		logger.Warn("Unauthorized access attempt", fields...)
	case appErr.IsValidation():
		logger.Info("Validation error", fields...)
	case appErr.IsNotFound():
		logger.Info("Resource not found", fields...)
	default:
		logger.Error("Application error occurred", fields...)
	}
}

// getRequestID получает ID запроса из контекста
func getRequestID(c *gin.Context) string {
	if requestID, exists := c.Get("request_id"); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return "unknown"
}

// getUserID получает ID пользователя из контекста
func getUserID(c *gin.Context) int64 {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(int64); ok {
			return id
		}
	}
	return 0
}

// HandleErrorWrapper оборачивает обработчики для автоматической обработки ошибок
func HandleErrorWrapper(logger *zap.Logger) func(gin.HandlerFunc) gin.HandlerFunc {
	return func(handler gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			// Выполняем обработчик
			handler(c)

			// Проверяем, есть ли ошибка в контексте
			if len(c.Errors) > 0 {
				err := c.Errors.Last().Err

				// Если это уже AppError, используем её
				if appErr, ok := errors.AsAppError(err); ok {
					sendErrorResponse(c, appErr, logger)
					return
				}

				// Иначе оборачиваем в AppError
				appErr := errors.Wrap(err, errors.ErrCodeInternal, "Handler error occurred").
					WithRequestID(getRequestID(c)).
					WithUserID(getUserID(c))

				sendErrorResponse(c, appErr, logger)
			}
		}
	}
}

// ValidationErrorResponse представляет ответ с ошибками валидации
type ValidationErrorResponse struct {
	Success   bool              `json:"success"`
	Errors    []errors.AppError `json:"errors"`
	Timestamp time.Time         `json:"timestamp"`
	RequestID string            `json:"request_id"`
}

// SendValidationErrors отправляет множественные ошибки валидации
func SendValidationErrors(c *gin.Context, validationErrors []errors.AppError, logger *zap.Logger) {
	requestID := getRequestID(c)

	// Добавляем контекст к каждой ошибке
	for i := range validationErrors {
		validationErrors[i].WithRequestID(requestID).
			WithContext("path", c.Request.URL.Path).
			WithContext("method", c.Request.Method)
	}

	response := ValidationErrorResponse{
		Success:   false,
		Errors:    validationErrors,
		Timestamp: time.Now(),
		RequestID: requestID,
	}

	// Логируем ошибки валидации
	logger.Info("Validation errors",
		zap.String("request_id", requestID),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.Int("error_count", len(validationErrors)),
	)

	c.JSON(http.StatusBadRequest, response)
}
