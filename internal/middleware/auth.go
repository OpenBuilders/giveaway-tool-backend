package middleware

import (
	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"
	"net/http"
	"os"
	"time"
)

func InitDataMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		initDataQuery := c.GetHeader("init_data")
		if initDataQuery != "" {
			token := os.Getenv("BOT_TOKEN")
			expIn := time.Minute * 20

			if os.Getenv("DEBUG") == "true" {
				expIn = time.Hour * 5000
			}

			if err := initdata.Validate(initDataQuery, token, expIn); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to validate init data"})
				c.Abort()
				return
			}

			parsedData, err := initdata.Parse(initDataQuery)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse init data"})
				c.Abort()
				return
			}

			c.Set("user", parsedData.User)
		}

		c.Next()
	}
}
