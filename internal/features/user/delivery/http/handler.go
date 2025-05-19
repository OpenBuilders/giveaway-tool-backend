package http

import (
	"fmt"
	"giveaway-tool-backend/internal/common/middleware"
	"giveaway-tool-backend/internal/features/user/models"
	"giveaway-tool-backend/internal/features/user/service"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"
)

type UserHandler struct {
	service service.UserService
}

func NewUserHandler(service service.UserService) *UserHandler {
	return &UserHandler{
		service: service,
	}
}

func (h *UserHandler) RegisterRoutes(router *gin.RouterGroup) {
	users := router.Group("/users")
	{
		users.GET("/me", h.getMe)
		users.GET("/:id", h.GetUser)
	}

	// Админские маршруты
	admin := router.Group("/users")
	admin.Use(middleware.RequireAdmin())
	{
		admin.PUT("/:id/status", h.UpdateUserStatus)
	}
}

// @Summary Get current user
// @Description Get or create current user based on Telegram init data. If user exists, updates their information if changed.
// @Tags users
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {object} models.UserResponse "User data"
// @Failure 400 {object} models.ErrorResponse "Invalid init data"
// @Failure 401 {object} models.ErrorResponse "Missing init data"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /users/me [get]
func (h *UserHandler) getMe(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Telegram Init Data required"})
		return
	}

	telegramUser, ok := user.(initdata.User)
	if !ok {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid user data format"})
		return
	}

	if os.Getenv("DEBUG") == "true" {
		fmt.Printf("[DEBUG] Creating/updating user: ID=%d, Username=%s\n", telegramUser.ID, telegramUser.Username)
	}

	userResponse, err := h.service.GetOrCreateUser(c.Request.Context(), telegramUser.ID, telegramUser.Username, telegramUser.FirstName, telegramUser.LastName)
	if err != nil {
		if os.Getenv("DEBUG") == "true" {
			fmt.Printf("[ERROR] Failed to get/create user: %v\n", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if os.Getenv("DEBUG") == "true" {
		fmt.Printf("[DEBUG] User response: %+v\n", userResponse)
	}

	c.JSON(http.StatusOK, userResponse)
}

// @Summary Get user by ID
// @Description Get user information by ID
// @Tags users
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path int true "User ID"
// @Success 200 {object} models.UserResponse "User data"
// @Failure 400 {object} models.ErrorResponse "Invalid request"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 403 {object} models.ErrorResponse "Forbidden - user is banned"
// @Failure 404 {object} models.ErrorResponse "User not found"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /users/{id} [get]
func (h *UserHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	user, err := h.service.GetUser(c.Request.Context(), id)
	if err != nil {
		if err == service.ErrUserNotFound {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// @Summary Update user status
// @Description Update user status (admin only)
// @Tags users
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path int true "User ID"
// @Param status body models.StatusUpdate true "New status"
// @Success 200 {object} models.UserResponse "Updated user data"
// @Failure 400 {object} models.ErrorResponse "Invalid request"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 403 {object} models.ErrorResponse "Forbidden - not an admin"
// @Failure 404 {object} models.ErrorResponse "User not found"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /users/{id}/status [put]
func (h *UserHandler) UpdateUserStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	var input models.StatusUpdate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateUserStatus(c.Request.Context(), id, input.Status); err != nil {
		if err == service.ErrUserNotFound {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user, err := h.service.GetUser(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}
