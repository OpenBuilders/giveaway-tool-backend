package http

import (
	"github.com/gofiber/fiber/v2"
)

// RequirementsHandlers exposes available requirement types for the client.
type RequirementsHandlers struct{}

func NewRequirementsHandlers() *RequirementsHandlers { return &RequirementsHandlers{} }

func (h *RequirementsHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/requirements/types", h.listTypes)
}

func (h *RequirementsHandlers) listTypes(c *fiber.Ctx) error {
	return c.JSON([]fiber.Map{
		{"type": "subscription", "description": "User must be a member of specified channels"},
	})
}
