package http

import (
	"github.com/gofiber/fiber/v2"
	tg "github.com/open-builders/giveaway-backend/internal/service/telegram"
)

// ChannelHandlers exposes channel-related endpoints backed by Telegram client.
type ChannelHandlers struct{ tg *tg.Client }

func NewChannelHandlers(tgc *tg.Client) *ChannelHandlers { return &ChannelHandlers{tg: tgc} }

func (h *ChannelHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/channels/:username/info", h.getChannelInfo)
	r.Get("/channels/:chat/membership", h.checkMembership)
	r.Get("/channels/:chat/boost", h.checkBoost)
}

func (h *ChannelHandlers) getChannelInfo(c *fiber.Ctx) error {
	username := c.Params("username")
	info, err := h.tg.GetPublicChannelInfo(c.Context(), username)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(info)
}

func (h *ChannelHandlers) checkMembership(c *fiber.Ctx) error {
	chat := c.Params("chat")
	userID, err := c.QueryInt("user_id", 0), error(nil)
	if userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing user_id"})
	}
	ok, err := h.tg.CheckMembership(c.Context(), int64(userID), chat)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": ok})
}

func (h *ChannelHandlers) checkBoost(c *fiber.Ctx) error {
	chat := c.Params("chat")
	userID := c.QueryInt("user_id", 0)
	if userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing user_id"})
	}
	ok, err := h.tg.CheckBoost(c.Context(), int64(userID), chat)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": ok})
}
