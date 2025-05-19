package middleware

import (
	"giveaway-tool-backend/internal/features/user/service"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"
)

func getAdminIDs() []int64 {
	adminIDsStr := os.Getenv("ADMIN_IDS")
	if adminIDsStr == "" {
		return []int64{}
	}

	var adminIDs []int64
	for _, idStr := range strings.Split(adminIDsStr, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64); err == nil {
			adminIDs = append(adminIDs, id)
		}
	}
	return adminIDs
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Telegram Init Data required"})
			return
		}

		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	adminIDs := getAdminIDs()

	return func(c *gin.Context) {
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

		isAdmin := false
		for _, adminID := range adminIDs {
			if telegramUser.ID == adminID {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			return
		}

		c.Next()
	}
}

func CheckBanned(userService service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.Next()
			return
		}

		telegramUser, ok := user.(initdata.User)
		if !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid user data format"})
			return
		}

		// Skip check for admins
		adminIDs := getAdminIDs()
		for _, adminID := range adminIDs {
			if telegramUser.ID == adminID {
				c.Next()
				return
			}
		}

		// Get user and check status
		u, err := userService.GetUser(c.Request.Context(), telegramUser.ID)
		if err == nil && u.Status == "banned" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Your account has been banned"})
			return
		}

		c.Next()
	}
}
