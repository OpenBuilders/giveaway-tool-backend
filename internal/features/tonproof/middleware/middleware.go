package middleware

import (
	"giveaway-tool-backend/internal/features/tonproof/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireTONVerification создает middleware, которое проверяет верификацию TON Proof
func RequireTONVerification(service *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("user_id")
		if userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		verified, err := service.IsVerified(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check verification status"})
			c.Abort()
			return
		}

		if !verified {
			c.JSON(http.StatusForbidden, gin.H{"error": "TON wallet not verified"})
			c.Abort()
			return
		}

		c.Next()
	}
}
