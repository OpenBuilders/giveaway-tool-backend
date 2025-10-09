package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	domain "github.com/your-org/giveaway-backend/internal/domain/user"
	mw "github.com/your-org/giveaway-backend/internal/http/middleware"
	"github.com/your-org/giveaway-backend/internal/service"
)

// UserHandlersFiber wires Fiber endpoints to the UserService.
type UserHandlersFiber struct {
	service *service.UserService
}

func NewUserHandlersFiber(svc *service.UserService) *UserHandlersFiber {
	return &UserHandlersFiber{service: svc}
}

// RegisterFiber registers routes on a Fiber router (app or group).
func (h *UserHandlersFiber) RegisterFiber(r fiber.Router) {
	r.Get("/users", h.listUsers)
	r.Post("/users", h.createOrUpdateUser)
	r.Get("/users/me", h.getMe)
	r.Get("/users/:id", h.getUserByID)
	r.Delete("/users/:id", h.deleteUser)
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

	firstName, _ := c.Locals(mw.FirstNameCtxParam).(string)
	lastName, _ := c.Locals(mw.LastNameCtxParam).(string)
	username, _ := c.Locals(mw.UsernameCtxParam).(string)

	role := "user"
	if u, err := h.service.GetByID(c.Context(), userID); err == nil && u != nil && u.Role != "" {
		role = u.Role
	}

	type meResponse struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
		Role      string `json:"role"`
	}

	return c.JSON(meResponse{
		ID:        userID,
		FirstName: firstName,
		LastName:  lastName,
		Username:  username,
		Role:      role,
	})
}
