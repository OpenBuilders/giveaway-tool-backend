package http

import (
	"time"

	"github.com/gofiber/fiber/v2"

	dg "github.com/your-org/giveaway-backend/internal/domain/giveaway"
	"github.com/your-org/giveaway-backend/internal/http/middleware"
	chsvc "github.com/your-org/giveaway-backend/internal/service/channels"
	gsvc "github.com/your-org/giveaway-backend/internal/service/giveaway"
	tgsvc "github.com/your-org/giveaway-backend/internal/service/telegram"
)

// GiveawayHandlersFiber provides Fiber endpoints for giveaways.
type GiveawayHandlersFiber struct {
	service  *gsvc.Service
	channels *chsvc.Service
	telegram *tgsvc.Client
}

func NewGiveawayHandlersFiber(svc *gsvc.Service, chs *chsvc.Service, tg *tgsvc.Client) *GiveawayHandlersFiber {
	return &GiveawayHandlersFiber{service: svc, channels: chs, telegram: tg}
}

func (h *GiveawayHandlersFiber) RegisterFiber(r fiber.Router) {
	r.Post("/giveaways", h.create)
	r.Get("/giveaways/:id", h.getByID)
	r.Get("/users/:creator_id/giveaways", h.listByCreator)
	r.Get("/giveaways", h.listActive)
	r.Get("/users/:creator_id/giveaways/finished", h.listFinishedByCreator)
	// Current user convenience endpoints
	r.Get("/giveaways/me/all", h.listMineAll)
	r.Patch("/giveaways/:id/status", h.updateStatus)
	r.Delete("/giveaways/:id", h.delete)
	r.Post("/giveaways/:id/join", h.join)
	r.Get("/prizes/templates", h.listPrizeTemplates)
}

type createPrizeReq struct {
	Place       *int   `json:"place,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Quantity    int    `json:"quantity,omitempty"`
}

type createSponsorReq struct {
	ID int64 `json:"id"`
}

type createGiveawayReq struct {
	Title           string             `json:"title"`
	Duration        int64              `json:"duration"`
	WinnersCount    int                `json:"winners_count"`
	Prizes          []createPrizeReq   `json:"prizes"`
	Description     string             `json:"description,omitempty"`
	MaxParticipants *int               `json:"max_participants,omitempty"`
	Requirements    []interface{}      `json:"requirements,omitempty"`
	Sponsors        []createSponsorReq `json:"sponsors,omitempty"`
}

// create handles creation of a new giveaway.
func (h *GiveawayHandlersFiber) create(c *fiber.Ctx) error {
	var req createGiveawayReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}

	// Build domain model
	now := time.Now().UTC()
	g := dg.Giveaway{
		Title:           req.Title,
		Description:     req.Description,
		StartedAt:       now,
		EndsAt:          now.Add(time.Duration(req.Duration) * time.Second),
		Duration:        req.Duration,
		MaxWinnersCount: req.WinnersCount,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Force creator from Telegram init-data context
	if userIDVal := c.Locals(middleware.UserIdCtxParam); userIDVal != nil {
		if id, ok := userIDVal.(int64); ok {
			g.CreatorID = id
		}
	}

	// Map prizes
	for _, p := range req.Prizes {
		qty := p.Quantity
		if qty <= 0 {
			qty = 1
		}
		g.Prizes = append(g.Prizes, dg.PrizePlace{
			Place:       p.Place,
			Title:       p.Title,
			Description: p.Description,
			Quantity:    qty,
		})
	}

	// Map sponsors: resolve full info by id using Telegram API, fallback to Redis cache
	for _, s := range req.Sponsors {
		if s.ID == 0 {
			g.Sponsors = append(g.Sponsors, dg.ChannelInfo{ID: s.ID})
			continue
		}
		// Try Telegram API by ID
		if h.telegram != nil {
			if info, err := h.telegram.GetPublicChannelInfoByID(c.Context(), s.ID); err == nil && info != nil {
				g.Sponsors = append(g.Sponsors, dg.ChannelInfo{
					ID:        info.ID,
					Title:     info.Title,
					Username:  info.Username,
					URL:       info.ChannelURL,
					AvatarURL: info.AvatarURL,
				})
				continue
			}
		}
		// Fallback to Redis cache
		if h.channels != nil {
			ch, _ := h.channels.GetByID(c.Context(), s.ID)
			if ch != nil {
				var url string
				if ch.Username != "" {
					url = "https://t.me/" + ch.Username
				}
				g.Sponsors = append(g.Sponsors, dg.ChannelInfo{ID: ch.ID, Title: ch.Title, Username: ch.Username, URL: url})
				continue
			}
		}
		g.Sponsors = append(g.Sponsors, dg.ChannelInfo{ID: s.ID})
	}

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

// listPrizeTemplates returns the available prize templates for the frontend.
func (h *GiveawayHandlersFiber) listPrizeTemplates(c *fiber.Ctx) error {
	type prizeTemplate struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}

	templates := []prizeTemplate{
		{Name: "Custom", Description: "Free-form custom prize", Type: "custom"},
	}

	return c.JSON(templates)
}

func (h *GiveawayHandlersFiber) listActive(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)
	minParticipants := c.QueryInt("min_participants", 0)
	list, err := h.service.ListActive(c.Context(), limit, offset, minParticipants)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}

// listMineAll returns all giveaways created by the current user (any status).
func (h *GiveawayHandlersFiber) listMineAll(c *fiber.Ctx) error {
	// user id from Telegram init-data middleware
	userIDAny := c.Locals(middleware.UserIdCtxParam)
	userID, _ := userIDAny.(int64)
	if userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	limit := c.QueryInt("limit", 100)
	offset := c.QueryInt("offset", 0)
	list, err := h.service.ListByCreator(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(list)
}
