package http

import (
	"github.com/gofiber/fiber/v2"
)

// RequirementsHandlers exposes available requirement types for the client.
type RequirementsHandlers struct{}

func NewRequirementsHandlers() *RequirementsHandlers { return &RequirementsHandlers{} }

func (h *RequirementsHandlers) RegisterFiber(r fiber.Router) {
	r.Get("/requirements/templates", h.listTemplates)
}

func (h *RequirementsHandlers) listTemplates(c *fiber.Ctx) error {
	return c.JSON([]fiber.Map{
		{"type": "subscription", "name": "Channel Subscription", "description": "User must be a member of specified channels"},
		{"type": "boost", "name": "Channel Boost", "description": "User must have active boost in specified channels"},
	})
}
