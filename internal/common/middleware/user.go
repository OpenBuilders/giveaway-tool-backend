package middleware

import (
	"fmt"
	"giveaway-tool-backend/internal/features/user/service"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"
)

func AutoCreateUser(userService service.UserService) gin.HandlerFunc {
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

		if os.Getenv("DEBUG") == "true" {
			fmt.Printf("[DEBUG] Auto-creating/updating user: ID=%d, Username=%s\n", telegramUser.ID, telegramUser.Username)
		}

		_, err := userService.GetOrCreateUser(c.Request.Context(), telegramUser.ID, telegramUser.Username, telegramUser.FirstName, telegramUser.LastName)
		if err != nil {
			if os.Getenv("DEBUG") == "true" {
				fmt.Printf("[ERROR] Failed to auto-create/update user: %v\n", err)
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to create/update user"})
			return
		}

		c.Next()
	}
}
