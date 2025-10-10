package http

import (
	"github.com/gofiber/fiber/v2"
	tgsvc "github.com/open-builders/giveaway-backend/internal/service/telegram"
)

// RequirementsHandlers exposes available requirement types for the client.
type RequirementsHandlers struct {
	telegram *tgsvc.Client
}

func NewRequirementsHandlers(tg *tgsvc.Client) *RequirementsHandlers {
	return &RequirementsHandlers{telegram: tg}
}

func (h *RequirementsHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/requirements/templates", h.listTemplates)
	r.Post("/requirements/channels/check-bulk", h.checkBotMembershipBulk)
}

func (h *RequirementsHandlers) listTemplates(c *fiber.Ctx) error {
	return c.JSON([]fiber.Map{
		{"type": "subscription", "name": "Channel Subscription", "description": "User must be a member of specified channels"},
		{"type": "boost", "name": "Channel Boost", "description": "User must have active boost in specified channels"},
	})
}

type checkBulkRequest struct {
	Usernames []string `json:"usernames"`
}

type checkBulkItem struct {
	Username string `json:"username"`
	Ok       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Channel  struct {
		ID       int64  `json:"id"`
		Type     string `json:"type"`
		Title    string `json:"title"`
		Username string `json:"username"`
	} `json:"channel"`
	BotStatus struct {
		Status          string `json:"status"`
		CanCheckMembers bool   `json:"can_check_members"`
	} `json:"bot_status"`
}

func (h *RequirementsHandlers) checkBotMembershipBulk(c *fiber.Ctx) error {
	if h.telegram == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "telegram client not configured"})
	}
	var req checkBulkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	if len(req.Usernames) == 0 {
		return c.JSON([]checkBulkItem{})
	}
	out := make([]checkBulkItem, 0, len(req.Usernames))
	for _, uname := range req.Usernames {
		if uname == "" {
			out = append(out, checkBulkItem{Username: uname, Ok: false, Error: "empty username"})
			continue
		}
		// Normalize to @username format for TG API helper
		chat := uname
		if chat[0] != '@' {
			chat = "@" + chat
		}
		// Fetch channel public info
		ch, errInfo := h.telegram.GetPublicChannelInfo(c.Context(), chat)
		item := checkBulkItem{Username: uname}
		if errInfo == nil && ch != nil {
			item.Channel.ID = ch.ID
			item.Channel.Type = ch.Type
			item.Channel.Title = ch.Title
			item.Channel.Username = ch.Username
		}
		// Check membership
		ok, err := h.telegram.IsBotMember(c.Context(), chat)
		if err != nil {
			item.Ok = false
			item.Error = err.Error()
		} else {
			item.Ok = ok
		}
		// Bot status details
		if status, can, err := h.telegram.GetBotMemberStatus(c.Context(), chat); err == nil {
			item.BotStatus.Status = status
			item.BotStatus.CanCheckMembers = can
		}
		out = append(out, item)
	}
	return c.JSON(fiber.Map{"results": out})
}
