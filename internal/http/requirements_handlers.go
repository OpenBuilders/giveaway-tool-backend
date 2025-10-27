package http

import (
	"math/big"

	"github.com/gofiber/fiber/v2"
	mw "github.com/open-builders/giveaway-backend/internal/http/middleware"
	tgsvc "github.com/open-builders/giveaway-backend/internal/service/telegram"
	tonb "github.com/open-builders/giveaway-backend/internal/service/tonbalance"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
)

// RequirementsHandlers exposes available requirement types for the client.
type RequirementsHandlers struct {
	telegram *tgsvc.Client
	users    *usersvc.Service
	ton      *tonb.Service
}

func NewRequirementsHandlers(tg *tgsvc.Client, users *usersvc.Service, ton *tonb.Service) *RequirementsHandlers {
	return &RequirementsHandlers{telegram: tg, users: users, ton: ton}
}

func (h *RequirementsHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/requirements/templates", h.listTemplates)
	r.Post("/requirements/channels/check-bulk", h.checkBotMembershipBulk)
	r.Post("/requirements/holdton/check", h.checkHoldTON)
	r.Post("/requirements/holdjetton/check", h.checkHoldJetton)
}

func (h *RequirementsHandlers) listTemplates(c *fiber.Ctx) error {
	return c.JSON([]fiber.Map{
		{"type": "subscription", "name": "Channel Subscription", "description": "User must be a member of specified channels"},
		{"type": "boost", "name": "Channel Boost", "description": "User must have active boost in specified channels"},
		{"type": "holdton", "name": "Hold TON", "description": "User must hold minimum TON balance"},
		{"type": "holdjetton", "name": "Hold Jetton", "description": "User must hold minimum amount of specified jetton"},
		{"type": "custom", "name": "Custom", "description": "User must fulfill custom requirement"},
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

// hold TON check
type holdTonRequest struct {
	TonMinBalanceNano int64 `json:"ton_min_balance_nano"`
}

func (h *RequirementsHandlers) checkHoldTON(c *fiber.Ctx) error {
	if h.users == nil || h.ton == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ton service not configured"})
	}
	// current user id from init-data
	userID := mw.GetUserID(c)
	if userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var req holdTonRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	u, err := h.users.GetByID(c.Context(), userID)
	if err != nil || u == nil || u.WalletAddress == "" {
		return c.JSON(fiber.Map{"ok": false, "error": "wallet not linked"})
	}
	bal, err := h.ton.GetAddressBalanceNano(c.Context(), u.WalletAddress)
	if err != nil {
		return c.JSON(fiber.Map{"ok": false, "error": err.Error()})
	}
	ok := req.TonMinBalanceNano <= 0 || bal >= req.TonMinBalanceNano
	return c.JSON(fiber.Map{"ok": ok, "balance_nano": bal})
}

// hold Jetton check
type holdJettonRequest struct {
	JettonAddress   string `json:"jetton_address"`
	JettonMinAmount int64  `json:"jetton_min_amount"`
}

func (h *RequirementsHandlers) checkHoldJetton(c *fiber.Ctx) error {
	if h.users == nil || h.ton == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ton service not configured"})
	}
	userID := mw.GetUserID(c)
	if userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var req holdJettonRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	if req.JettonAddress == "" || req.JettonMinAmount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid jetton requirement"})
	}
	u, err := h.users.GetByID(c.Context(), userID)
	if err != nil || u == nil || u.WalletAddress == "" {
		return c.JSON(fiber.Map{"ok": false, "error": "wallet not linked"})
	}
	bal, err := h.ton.GetJettonBalanceNano(c.Context(), u.WalletAddress, req.JettonAddress)
	if err != nil {
		return c.JSON(fiber.Map{"ok": false, "error": err.Error()})
	}
	// Convert human-entered jetton amount to smallest units using decimals
	dec, derr := h.ton.GetJettonDecimals(c.Context(), req.JettonAddress)
	if derr != nil {
		return c.JSON(fiber.Map{"ok": false, "error": derr.Error()})
	}
	// big-int conversion: req.JettonMinAmount * 10^dec
	reqSmall := new(big.Int).SetInt64(req.JettonMinAmount)
	pow10 := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dec)), nil)
	reqSmall.Mul(reqSmall, pow10)
	balBI := new(big.Int).SetInt64(bal)
	ok := balBI.Cmp(reqSmall) >= 0
	return c.JSON(fiber.Map{"ok": ok, "balance_nano": bal})
}
