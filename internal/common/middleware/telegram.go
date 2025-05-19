package middleware

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"
)

func TelegramInitDataMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		initDataQuery := c.GetHeader("init_data")
		if initDataQuery == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Telegram Init Data required"})
			return
		}

		if os.Getenv("DEBUG") == "true" {
			fmt.Printf("[DEBUG] Raw Init Data received: %s\n", initDataQuery)
		}

		token := os.Getenv("BOT_TOKEN")
		if token == "" {
			if os.Getenv("DEBUG") == "true" {
				fmt.Printf("[ERROR] BOT_TOKEN not set in environment\n")
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Server configuration error"})
			return
		}

		// Disable expiration check
		expIn := time.Duration(0)

		if err := initdata.Validate(initDataQuery, token, expIn); err != nil {
			if os.Getenv("DEBUG") == "true" {
				fmt.Printf("[ERROR] Validation failed: %v\n", err)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("Invalid init data: %v", err)})
			return
		}

		parsedData, err := initdata.Parse(initDataQuery)
		if err != nil {
			if os.Getenv("DEBUG") == "true" {
				fmt.Printf("[ERROR] Parse failed: %v\n", err)
			}
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to parse init data: %v", err)})
			return
		}

		if os.Getenv("DEBUG") == "true" {
			fmt.Printf("[DEBUG] Successfully validated and parsed init data\n")
			fmt.Printf("[DEBUG] User: %+v\n", parsedData.User)
		}

		c.Set("user", parsedData.User)
		c.Next()
	}
}
