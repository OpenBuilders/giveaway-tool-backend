package http

import (
	"github.com/gofiber/fiber/v2"
	tg "github.com/your-org/giveaway-backend/internal/service/telegram"
)

// ChannelHandlers exposes channel-related endpoints backed by Telegram client.
type ChannelHandlers struct{ tg *tg.Client }

func NewChannelHandlers(tgc *tg.Client) *ChannelHandlers { return &ChannelHandlers{tg: tgc} }

func (h *ChannelHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/channels/:username/info", h.getChannelInfo)
}

func (h *ChannelHandlers) getChannelInfo(c *fiber.Ctx) error {
	username := c.Params("username")
	info, err := h.tg.GetPublicChannelInfo(c.Context(), username)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(info)
}
