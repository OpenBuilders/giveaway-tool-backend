package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	dg "github.com/your-org/giveaway-backend/internal/domain/giveaway"
	"github.com/your-org/giveaway-backend/internal/http/middleware"
	gsvc "github.com/your-org/giveaway-backend/internal/service/giveaway"
)

// GiveawayHandlersFiber provides Fiber endpoints for giveaways.
type GiveawayHandlersFiber struct{ service *gsvc.Service }

func NewGiveawayHandlersFiber(svc *gsvc.Service) *GiveawayHandlersFiber {
	return &GiveawayHandlersFiber{service: svc}
}

func (h *GiveawayHandlersFiber) RegisterFiber(r fiber.Router) {
	r.Post("/giveaways", h.create)
	r.Get("/giveaways/:id", h.getByID)
	r.Get("/users/:creator_id/giveaways", h.listByCreator)
	r.Get("/giveaways", h.listActive)
	r.Get("/users/:creator_id/giveaways/finished", h.listFinishedByCreator)
	r.Patch("/giveaways/:id/status", h.updateStatus)
	r.Delete("/giveaways/:id", h.delete)
	r.Post("/giveaways/:id/join", h.join)
}

// create handles creation of a new giveaway.
func (h *GiveawayHandlersFiber) create(c *fiber.Ctx) error {
	var g dg.Giveaway
	if err := c.BodyParser(&g); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	// Force creator from Telegram init-data context
	if userIDVal := c.Locals(middleware.UserIdCtxParam); userIDVal != nil {
		if id, ok := userIDVal.(int64); ok {
			g.CreatorID = id
		}
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	g.UpdatedAt = time.Now().UTC()

	id, err := h.service.Create(c.Context(), &g)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (h *GiveawayHandlersFiber) getByID(c *fiber.Ctx) error {
	id := c.Params("id")
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(g)
}

func (h *GiveawayHandlersFiber) listByCreator(c *fiber.Ctx) error {
	creatorID, err := c.ParamsInt("creator_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid creator_id"})
	}
	limit := c.QueryInt("limit", 100)
	offset := c.QueryInt("offset", 0)
	list, err := h.service.ListByCreator(c.Context(), int64(creatorID), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}

type updateStatusReq struct {
	Status dg.GiveawayStatus `json:"status"`
}

func (h *GiveawayHandlersFiber) updateStatus(c *fiber.Ctx) error {
	id := c.Params("id")
	var body updateStatusReq
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	if err := h.service.UpdateStatus(c.Context(), id, body.Status); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *GiveawayHandlersFiber) delete(c *fiber.Ctx) error {
	id := c.Params("id")
	// requester from middleware
	requesterIDAny := c.Locals(middleware.UserIdCtxParam)
	requesterID, _ := requesterIDAny.(int64)
	if requesterID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if err := h.service.Delete(c.Context(), id, requesterID); err != nil {
		switch err.Error() {
		case "not found":
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		case "forbidden":
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *GiveawayHandlersFiber) join(c *fiber.Ctx) error {
	id := c.Params("id")
	requesterIDAny := c.Locals(middleware.UserIdCtxParam)
	requesterID, _ := requesterIDAny.(int64)
	if requesterID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if err := h.service.Join(c.Context(), id, requesterID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *GiveawayHandlersFiber) listFinishedByCreator(c *fiber.Ctx) error {
	creatorID, err := c.ParamsInt("creator_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid creator_id"})
	}
	limit := c.QueryInt("limit", 100)
	offset := c.QueryInt("offset", 0)
	list, err := h.service.ListFinishedByCreator(c.Context(), int64(creatorID), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}

func (h *GiveawayHandlersFiber) listActive(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)
	minParticipants := c.QueryInt("min_participants", 1)
	list, err := h.service.ListActive(c.Context(), limit, offset, minParticipants)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}
