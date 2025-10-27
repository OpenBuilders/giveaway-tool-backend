package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	mw "github.com/open-builders/giveaway-backend/internal/http/middleware"
	tp "github.com/open-builders/giveaway-backend/internal/service/tonproof"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
)

// TonProofHandlers provides public endpoints compatible with ton-connect demo backend
type TonProofHandlers struct {
	svc    *tp.Service
	domain string
	users  *usersvc.Service
}

func NewTonProofHandlers(s *tp.Service, domain string, users *usersvc.Service) *TonProofHandlers {
	return &TonProofHandlers{svc: s, domain: domain, users: users}
}

// RegisterFiber registers routes; place under router with Telegram init-data auth middleware
func (h *TonProofHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/ton-proof/generatePayload", h.generatePayload)
	r.Post("/ton-proof/checkProof", h.checkProof)
}

type payloadResp struct {
	TonProof struct {
		Timestamp int64 `json:"timestamp"`
		Domain    struct {
			LengthBytes int    `json:"lengthBytes"`
			Value       string `json:"value"`
		} `json:"domain"`
		Payload string `json:"payload"`
	} `json:"ton_proof"`
}

func (h *TonProofHandlers) generatePayload(c *fiber.Ctx) error {
	if h.svc == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "tonproof not configured"})
	}
	payload, err := h.svc.GeneratePayload(c.Context(), "public")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	var resp payloadResp
	resp.TonProof.Timestamp = time.Now().Unix()
	resp.TonProof.Domain.LengthBytes = len(h.domain)
	resp.TonProof.Domain.Value = h.domain
	resp.TonProof.Payload = payload
	return c.JSON(resp)
}

type checkReq struct {
	Address string            `json:"address"`
	Network string            `json:"network"`
	Proof   tp.TonProofObject `json:"proof"`
}

func (h *TonProofHandlers) checkProof(c *fiber.Ctx) error {
	if h.svc == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "tonproof not configured"})
	}
	var req checkReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	vr, err := h.svc.VerifyProof(c.Context(), &tp.VerifyRequest{Address: req.Address, Network: req.Network, Proof: req.Proof})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if !vr.Success {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "reason": vr.Reason})
	}
	// Save wallet to authenticated user
	userIDVal := c.Locals(mw.UserIdCtxParam)
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var userID int64
	switch v := userIDVal.(type) {
	case int64:
		userID = v
	case int:
		userID = int64(v)
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			userID = parsed
		}
	}
	if userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if h.users != nil && userID != 0 {
		err = h.users.UpdateWallet(c.Context(), userID, req.Address)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.JSON(fiber.Map{"success": true})
}
