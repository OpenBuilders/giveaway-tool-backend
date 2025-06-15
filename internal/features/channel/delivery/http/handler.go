package http

import (
	"fmt"
	"giveaway-tool-backend/internal/features/channel/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ChannelHandler struct {
	service service.ChannelService
}

func NewChannelHandler(service service.ChannelService) *ChannelHandler {
	return &ChannelHandler{
		service: service,
	}
}

func (h *ChannelHandler) RegisterRoutes(router *gin.RouterGroup) {
	channels := router.Group("/channels")
	{
		channels.GET("/me", h.getUserChannels)
		channels.GET("/:username/info", h.getPublicChannelInfo)
	}
}

type ChannelInfo struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	AvatarURL  string `json:"avatar_url"`
	ChannelURL string `json:"channel_url"`
}

func (h *ChannelHandler) getUserChannels(c *gin.Context) {
	userID := c.GetInt64("user_id")

	channels, err := h.service.GetUserChannels(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user channels"})
		return
	}

	response := make([]ChannelInfo, 0, len(channels))
	for _, channelID := range channels {
		title, err := h.service.GetChannelTitle(c.Request.Context(), channelID)
		if err != nil {
			continue
		}

		username, err := h.service.GetChannelUsername(c.Request.Context(), channelID)
		if err != nil || username == "" {
			continue
		}

		avatarURL := fmt.Sprintf("https://t.me/i/userpic/320/%s.jpg", username)

		response = append(response, ChannelInfo{
			ID:         channelID,
			Title:      title,
			AvatarURL:  avatarURL,
			ChannelURL: "https://t.me/" + username,
		})
	}

	c.JSON(http.StatusOK, gin.H{"channels": response})
}

func (h *ChannelHandler) getPublicChannelInfo(c *gin.Context) {
	username := c.Param("username")

	info, err := h.service.GetPublicChannelInfo(c.Request.Context(), username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get channel info"})
		return
	}

	c.JSON(http.StatusOK, info)
}
