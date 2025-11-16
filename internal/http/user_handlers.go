package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	domain "github.com/open-builders/giveaway-backend/internal/domain/user"
	mw "github.com/open-builders/giveaway-backend/internal/http/middleware"
	chsvc "github.com/open-builders/giveaway-backend/internal/service/channels"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
)

// UserHandlersFiber wires Fiber endpoints to the UserService.
type UserHandlersFiber struct {
	service  *usersvc.Service
	channels *chsvc.Service
}

func NewUserHandlersFiber(svc *usersvc.Service, ch *chsvc.Service) *UserHandlersFiber {
	return &UserHandlersFiber{service: svc, channels: ch}
}

// RegisterFiber registers routes on a Fiber router (app or group).
func (h *UserHandlersFiber) RegisterFiber(r fiber.Router) {
	// r.Get("/users", h.listUsers)
	// r.Post("/users", h.createOrUpdateUser)
	r.Get("/users/me", h.getMe)
	// r.Get("/users/:id", h.getUserByID)
	// r.Delete("/users/:id", h.deleteUser)
	r.Get("/users/me/channels", h.listUserChannels)
}

func (h *UserHandlersFiber) listUsers(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	users, err := h.service.List(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(users)
}

func (h *UserHandlersFiber) createOrUpdateUser(c *fiber.Ctx) error {
	var u domain.User
	if err := c.BodyParser(&u); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	u.UpdatedAt = time.Now().UTC()
	if err := h.service.Upsert(c.Context(), &u); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(u)
}

func (h *UserHandlersFiber) getUserByID(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	u, err := h.service.GetByID(c.Context(), int64(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if u == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(u)
}

func (h *UserHandlersFiber) deleteUser(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.service.Delete(c.Context(), int64(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// getMe returns the current user data extracted from Telegram init-data context.
// Role is resolved from the database if present; otherwise defaults to "user".
func (h *UserHandlersFiber) getMe(c *fiber.Ctx) error {
	userID := mw.GetUserID(c)
	if userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	firstName, _ := c.Locals(mw.FirstNameCtxParam).(string)
	lastName, _ := c.Locals(mw.LastNameCtxParam).(string)
	username, _ := c.Locals(mw.UsernameCtxParam).(string)
	photoURL, _ := c.Locals(mw.UserPicCtxParam).(string)
	isPremium, _ := c.Locals(mw.IsPremiumCtxParam).(bool)
	// Load existing user to preserve wallet and role if present
	walletAddress := ""
	role := "user"
	currentAvatar := ""
	currentIsPremium := false
	if u, err := h.service.GetByID(c.Context(), userID); err == nil && u != nil {
		if u.Role != "" {
			role = u.Role
		}
		if u.WalletAddress != "" {
			walletAddress = u.WalletAddress
		}
		if u.AvatarURL != "" {
			currentAvatar = u.AvatarURL
		}
		currentIsPremium = u.IsPremium
	}

	// Decide effective avatar to store/return
	avatarURL := currentAvatar
	if photoURL != "" && photoURL != currentAvatar {
		avatarURL = photoURL
	}
	// Effective premium flag: trust current init-data boolean; if missing, keep DB value
	effectivePremium := currentIsPremium
	if isPremium != currentIsPremium {
		effectivePremium = isPremium
	}

	// create or update user
	if err := h.service.Upsert(c.Context(), &domain.User{
		ID:            userID,
		FirstName:     firstName,
		LastName:      lastName,
		Username:      username,
		AvatarURL:     avatarURL,
		IsPremium:     effectivePremium,
		Role:          role,
		Status:        "active",
		WalletAddress: walletAddress,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	type meResponse struct {
		ID            int64  `json:"id"`
		FirstName     string `json:"first_name"`
		LastName      string `json:"last_name"`
		Username      string `json:"username"`
		AvatarURL     string `json:"avatar_url"`
		IsPremium     bool   `json:"is_premium"`
		Role          string `json:"role"`
		WalletAddress string `json:"wallet_address"`
	}

	return c.JSON(meResponse{
		ID:            userID,
		FirstName:     firstName,
		LastName:      lastName,
		Username:      username,
		AvatarURL:     avatarURL,
		IsPremium:     effectivePremium,
		Role:          role,
		WalletAddress: walletAddress,
	})
}

func (h *UserHandlersFiber) listUserChannels(c *fiber.Ctx) error {
	// Read user id from Telegram init-data middleware context
	userIDVal := c.Locals(mw.UserIdCtxParam)
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var id int64
	switch v := userIDVal.(type) {
	case int64:
		id = v
	case int:
		id = int64(v)
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			id = parsed
		}
	}
	if id == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.channels == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "channels service not configured"})
	}
	items, err := h.channels.ListUserChannels(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

// TON Proof-related functionality has been moved to dedicated public handlers.
