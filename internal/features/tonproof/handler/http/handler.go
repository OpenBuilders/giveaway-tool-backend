package http

import (
	"net/http"

	"giveaway-tool-backend/internal/features/tonproof/models"
	"giveaway-tool-backend/internal/features/tonproof/service"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service service.Service
}

func NewHandler(service service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	tonproof := router.Group("/tonproof")
	{
		tonproof.POST("/verify", h.VerifyProof)
		tonproof.GET("/status", h.CheckVerification)
	}
}

// @Summary Verify TON Proof
// @Description Verify TON Proof for a user
// @Tags tonproof
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param proof body models.TONProofRequest true "TON Proof data"
// @Success 200 {object} models.TONProofResponse
// @Failure 400 {object} models.ErrorResponse "Invalid request"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /tonproof/verify [post]
func (h *Handler) VerifyProof(c *gin.Context) {
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req models.TONProofRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := h.service.VerifyProof(c.Request.Context(), userID.(int64), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.TONProofResponse{
		Success: true,
		Message: "Successfully verified",
	})
}

// @Summary Check TON Proof status
// @Description Check if user's TON Proof is verified
// @Tags tonproof
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {object} map[string]bool "Verification status"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /tonproof/status [get]
func (h *Handler) CheckVerification(c *gin.Context) {
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	isVerified, err := h.service.IsVerified(c.Request.Context(), userID.(int64))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"is_verified": isVerified,
	})
}
