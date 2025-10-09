package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	domain "github.com/your-org/giveaway-backend/internal/domain/user"
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
