package http

import (
	"fmt"
	"io"
	"math/big"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	dg "github.com/open-builders/giveaway-backend/internal/domain/giveaway"
	"github.com/open-builders/giveaway-backend/internal/http/middleware"
	chsvc "github.com/open-builders/giveaway-backend/internal/service/channels"
	gsvc "github.com/open-builders/giveaway-backend/internal/service/giveaway"
	tgsvc "github.com/open-builders/giveaway-backend/internal/service/telegram"
	tonb "github.com/open-builders/giveaway-backend/internal/service/tonbalance"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
)

// GiveawayHandlersFiber provides Fiber endpoints for giveaways.
type GiveawayHandlersFiber struct {
	service  *gsvc.Service
	channels *chsvc.Service
	telegram *tgsvc.Client
	users    *usersvc.Service
	ton      *tonb.Service
}

func NewGiveawayHandlersFiber(svc *gsvc.Service, chs *chsvc.Service, tg *tgsvc.Client, users *usersvc.Service, ton *tonb.Service) *GiveawayHandlersFiber {
	return &GiveawayHandlersFiber{service: svc, channels: chs, telegram: tg, users: users, ton: ton}
}

func (h *GiveawayHandlersFiber) RegisterFiber(r fiber.Router) {
	r.Post("/giveaways", h.create)
	r.Get("/giveaways/:id", h.getByID)
	r.Get("/giveaways/:id/list-loaded-winners", h.listWinnersWithPrizes)
	r.Delete("/giveaways/:id/loaded-winners", h.clearLoadedWinners)
	r.Get("/giveaways/:id/check-requirements", h.checkRequirements)
	r.Get("/users/:creator_id/giveaways", h.listByCreator)
	r.Get("/giveaways", h.listActive)
	r.Get("/users/:creator_id/giveaways/finished", h.listFinishedByCreator)
	// Current user convenience endpoints
	r.Get("/giveaways/me/all", h.listMineAll)
	r.Patch("/giveaways/:id/status", h.updateStatus)
	r.Delete("/giveaways/:id", h.delete)
	r.Post("/giveaways/:id/join", h.join)
	// Manual winners upload (now returns preview-style response)
	r.Post("/giveaways/:id/manual-candidates", h.uploadManualCandidates)
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
	Title           string                 `json:"title"`
	Duration        int64                  `json:"duration"`
	WinnersCount    int                    `json:"winners_count"`
	Prizes          []createPrizeReq       `json:"prizes"`
	Description     string                 `json:"description,omitempty"`
	MaxParticipants *int                   `json:"max_participants,omitempty"`
	Requirements    []createRequirementReq `json:"requirements,omitempty"`
	Sponsors        []createSponsorReq     `json:"sponsors,omitempty"`
}

// createRequirementReq accepts flexible payloads from the client
// and is normalized into domain.Requirement.
type createRequirementReq struct {
	Type dg.RequirementType `json:"type"`
	// Client may send either "username" or "channel_username"
	Username        string `json:"username,omitempty"`
	ChannelUsername string `json:"channel_username,omitempty"`
	ChannelID       int64  `json:"channel_id,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	// On-chain
	TonMinBalanceNano int64  `json:"ton_min_balance_nano,omitempty"`
	JettonAddress     string `json:"jetton_address,omitempty"`
	JettonMinAmount   int64  `json:"jetton_min_amount,omitempty"`
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
	g.CreatorID = middleware.GetUserID(c)

	// Map and enrich requirements first (independent of prizes)
	for _, r := range req.Requirements {
		switch r.Type {
		case dg.RequirementTypeSubscription:
			uname := r.ChannelUsername
			if uname == "" {
				uname = r.Username
			}
			// normalize without @ for storage; telegram client accepts with @
			normalized := uname
			if len(normalized) > 0 && normalized[0] == '@' {
				normalized = normalized[1:]
			}
			reqEntry := dg.Requirement{Type: dg.RequirementTypeSubscription}
			if r.Name != "" {
				reqEntry.ChannelTitle = r.Name
			}
			if r.Description != "" {
				reqEntry.Description = r.Description
			}
			// Try Telegram enrichment
			if h.telegram != nil && normalized != "" {
				if info, err := h.telegram.GetPublicChannelInfo(c.Context(), normalized); err == nil && info != nil {
					reqEntry.ChannelID = info.ID
					reqEntry.ChannelUsername = info.Username
					reqEntry.ChannelTitle = info.Title
					reqEntry.ChannelURL = info.ChannelURL
					// prefer client-provided avatar if present
					if r.AvatarURL != "" {
						reqEntry.AvatarURL = r.AvatarURL
					} else {
						reqEntry.AvatarURL = info.AvatarURL
					}
				} else {
					// fallback: store username only when API fails
					reqEntry.ChannelUsername = normalized
					if r.ChannelID != 0 {
						reqEntry.ChannelID = r.ChannelID
					}
					if r.AvatarURL != "" {
						reqEntry.AvatarURL = r.AvatarURL
					}
				}
			} else {
				// No telegram client: store what we have
				reqEntry.ChannelUsername = normalized
				if r.ChannelID != 0 {
					reqEntry.ChannelID = r.ChannelID
				}
				if r.AvatarURL != "" {
					reqEntry.AvatarURL = r.AvatarURL
				}
			}
			g.Requirements = append(g.Requirements, reqEntry)
		case dg.RequirementTypeBoost:
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypeBoost, Description: r.Description})
		case dg.RequirementTypeCustom:
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypeCustom, ChannelTitle: r.Name, Description: r.Description})
		case dg.RequirementTypeHoldTON:
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypeHoldTON, TonMinBalanceNano: r.TonMinBalanceNano, Title: r.Name, Description: r.Description})
		case dg.RequirementTypeHoldJetton:
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypeHoldJetton, JettonAddress: r.JettonAddress, JettonMinAmount: r.JettonMinAmount, Title: r.Name, Description: r.Description})
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
	// compute user role
	var userRole string
	if uid := middleware.GetUserID(c); uid != 0 {
		if role, err := h.service.GetUserRole(c.Context(), g, uid); err == nil {
			userRole = role
		}
	}
	// Build DTO without creator_id but with user_role
	type requirementDTO struct {
		Name        string             `json:"name,omitempty"`
		Type        dg.RequirementType `json:"type"`
		Username    string             `json:"username,omitempty"`
		AvatarURL   string             `json:"avatar_url,omitempty"`
		Description string             `json:"description,omitempty"`
		// On-chain fields
		TonMinBalanceNano int64  `json:"ton_min_balance_nano,omitempty"`
		JettonAddress     string `json:"jetton_address,omitempty"`
		JettonMinAmount   int64  `json:"jetton_min_amount,omitempty"`
	}

	type GiveawayDTO struct {
		ID                string            `json:"id"`
		Title             string            `json:"title"`
		Description       string            `json:"description"`
		StartedAt         time.Time         `json:"started_at"`
		EndsAt            time.Time         `json:"ends_at"`
		Duration          int64             `json:"duration"`
		MaxWinnersCount   int               `json:"winners_count"`
		Status            dg.GiveawayStatus `json:"status"`
		CreatedAt         time.Time         `json:"created_at"`
		UpdatedAt         time.Time         `json:"updated_at"`
		Prizes            []dg.PrizePlace   `json:"prizes,omitempty"`
		Sponsors          []dg.ChannelInfo  `json:"sponsors"`
		Requirements      []requirementDTO  `json:"requirements,omitempty"`
		Winners           []dg.Winner       `json:"winners,omitempty"`
		ParticipantsCount int               `json:"participants_count"`
		UserRole          string            `json:"user_role,omitempty"`
	}
	// Map requirements to requested API shape
	reqs := make([]requirementDTO, 0, len(g.Requirements))
	for _, r := range g.Requirements {
		name := r.ChannelTitle
		if name == "" {
			name = r.Title
		}
		reqs = append(reqs, requirementDTO{
			Name:              name,
			Type:              r.Type,
			Username:          r.ChannelUsername,
			AvatarURL:         r.AvatarURL,
			Description:       r.Description,
			TonMinBalanceNano: r.TonMinBalanceNano,
			JettonAddress:     r.JettonAddress,
			JettonMinAmount:   r.JettonMinAmount,
		})
	}

	dto := GiveawayDTO{
		ID:                g.ID,
		Title:             g.Title,
		Description:       g.Description,
		StartedAt:         g.StartedAt,
		EndsAt:            g.EndsAt,
		Duration:          g.Duration,
		MaxWinnersCount:   g.MaxWinnersCount,
		Status:            g.Status,
		CreatedAt:         g.CreatedAt,
		UpdatedAt:         g.UpdatedAt,
		Prizes:            g.Prizes,
		Sponsors:          g.Sponsors,
		Requirements:      reqs,
		Winners:           g.Winners,
		ParticipantsCount: g.ParticipantsCount,
		UserRole:          userRole,
	}
	return c.JSON(dto)
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
	requesterID := middleware.GetUserID(c)
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

// requirementsAllMet checks all giveaway requirements for the current user and
// returns true only if every requirement is satisfied.
func (h *GiveawayHandlersFiber) requirementsAllMet(c *fiber.Ctx, g *dg.Giveaway) bool {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return false
	}
	allMet := true
	for _, rqm := range g.Requirements {
		res := h.checkSingleRequirement(c, userID, &rqm)
		if res.Error != "" || res.Status != "success" {
			allMet = false
			break
		}
	}
	return allMet
}

func (h *GiveawayHandlersFiber) join(c *fiber.Ctx) error {
	id := c.Params("id")
	requesterID := middleware.GetUserID(c)
	if requesterID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	// Ensure all requirements are satisfied before joining
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if !h.requirementsAllMet(c, g) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "requirements not satisfied"})
	}
	if err := h.service.Join(c.Context(), id, requesterID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *GiveawayHandlersFiber) uploadManualCandidates(c *fiber.Ctx) error {
	// Auth required; use giveaway id to filter by participants
	id := c.Params("id")
	creatorID := middleware.GetUserID(c)
	if creatorID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Load giveaway for role checks (participant/winner)
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	var content []byte
	if file, err := c.FormFile("file"); err == nil && file != nil {
		f, err := file.Open()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		content = b
	} else {
		content = c.Body()
	}
	// Support either newline-separated or comma-separated tokens
	raw := strings.ReplaceAll(string(content), ",", " ")
	tokens := strings.Fields(raw)
	if len(tokens) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no candidates"})
	}

	type previewItem struct {
		UserID    int64  `json:"user_id"`
		Username  string `json:"username"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Source    string `json:"source"`
	}

	out := make([]previewItem, 0, len(tokens))
	seen := make(map[int64]struct{})
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "@") {
			uname := strings.TrimPrefix(t, "@")
			if h.users != nil {
				if usr, uerr := h.users.GetByUsername(c.Context(), uname); uerr == nil && usr != nil {
					// keep only participants or winners
					if role, rerr := h.service.GetUserRole(c.Context(), g, usr.ID); rerr == nil && (role == "participant" || role == "winner") {
						if _, ok := seen[usr.ID]; ok {
							continue
						}
						seen[usr.ID] = struct{}{}
						fullName := strings.TrimSpace(strings.TrimSpace(usr.FirstName + " " + usr.LastName))
						avatar := ""
						if usr.Username != "" {
							avatar = "https://t.me/i/userpic/160/" + usr.Username + ".jpg"
						}
						out = append(out, previewItem{UserID: usr.ID, Username: usr.Username, Name: fullName, AvatarURL: avatar, Source: "username"})
					}
				}
			}
			continue
		}
		uid, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			continue
		}
		// Check user exists and participated
		var username, name, avatar string
		if h.users != nil {
			if usr, uerr := h.users.GetByID(c.Context(), uid); uerr == nil && usr != nil {
				if role, rerr := h.service.GetUserRole(c.Context(), g, uid); rerr == nil && (role == "participant" || role == "winner") {
					if _, ok := seen[uid]; ok {
						continue
					}
					seen[uid] = struct{}{}
					username = usr.Username
					name = strings.TrimSpace(strings.TrimSpace(usr.FirstName + " " + usr.LastName))
					if username != "" {
						avatar = "https://t.me/i/userpic/160/" + username + ".jpg"
					}
					out = append(out, previewItem{UserID: uid, Username: username, Name: name, AvatarURL: avatar, Source: "id"})
				}
			}
		}
	}

	// Enforce max winners limit from giveaway settings
	if g.MaxWinnersCount > 0 && len(out) > g.MaxWinnersCount {
		// shuffle to ensure random selection when truncating
		rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		out = out[:g.MaxWinnersCount]
	}

	// Store manual winners and distribute prizes, but keep giveaway pending
	winnerIDs := make([]int64, 0, len(out))
	for _, it := range out {
		winnerIDs = append(winnerIDs, it.UserID)
	}
	if err := h.service.SetManualWinners(c.Context(), id, creatorID, winnerIDs); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Return preview users with their assigned prizes
	winners, err := h.service.ListWinnersWithPrizes(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Build map user_id -> prizes for response join
	prizeByUser := make(map[int64][]dg.WinnerPrize, len(winners))
	for _, w := range winners {
		prizeByUser[w.UserID] = w.Prizes
	}
	type respItem struct {
		UserID    int64            `json:"user_id"`
		Username  string           `json:"username"`
		Name      string           `json:"name"`
		AvatarURL string           `json:"avatar_url"`
		Source    string           `json:"source"`
		Prizes    []dg.WinnerPrize `json:"prizes"`
	}
	resp := make([]respItem, 0, len(out))
	for _, it := range out {
		resp = append(resp, respItem{
			UserID:    it.UserID,
			Username:  it.Username,
			Name:      it.Name,
			AvatarURL: it.AvatarURL,
			Source:    it.Source,
			Prizes:    prizeByUser[it.UserID],
		})
	}
	return c.JSON(fiber.Map{"results": resp})
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

// listWinnersWithPrizes returns winners and their prizes for a giveaway, any status.
func (h *GiveawayHandlersFiber) listWinnersWithPrizes(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing id"})
	}
	// Optional: ensure giveaway exists
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	winners, err := h.service.ListWinnersWithPrizes(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Build same response format as uploadManualCandidates
	type respItem struct {
		UserID    int64            `json:"user_id"`
		Username  string           `json:"username"`
		Name      string           `json:"name"`
		AvatarURL string           `json:"avatar_url"`
		Source    string           `json:"source"`
		Prizes    []dg.WinnerPrize `json:"prizes"`
	}
	resp := make([]respItem, 0, len(winners))
	for _, w := range winners {
		var username, name, avatar string
		if h.users != nil {
			if usr, uerr := h.users.GetByID(c.Context(), w.UserID); uerr == nil && usr != nil {
				username = usr.Username
				name = strings.TrimSpace(strings.TrimSpace(usr.FirstName + " " + usr.LastName))
				if username != "" {
					avatar = "https://t.me/i/userpic/160/" + username + ".jpg"
				}
			}
		}
		resp = append(resp, respItem{
			UserID:    w.UserID,
			Username:  username,
			Name:      name,
			AvatarURL: avatar,
			Source:    "id",
			Prizes:    w.Prizes,
		})
	}
	return c.JSON(fiber.Map{"results": resp})
}

// clearLoadedWinners deletes loaded winners and their prizes; only creator and only if pending.
func (h *GiveawayHandlersFiber) clearLoadedWinners(c *fiber.Ctx) error {
	id := c.Params("id")
	creatorID := middleware.GetUserID(c)
	if creatorID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	// Validate giveaway and role
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if g.CreatorID != creatorID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	if g.Status != dg.GiveawayStatusPending {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "not pending"})
	}
	if err := h.service.ClearManualWinners(c.Context(), id, creatorID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
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
	userID := middleware.GetUserID(c)
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

// checkRequirements verifies whether the current user satisfies each requirement of a giveaway.
// Returns detailed results and overall all_met flag.
func (h *GiveawayHandlersFiber) checkRequirements(c *fiber.Ctx) error {
	if h.telegram == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "telegram client not configured"})
	}
	// Current user from Telegram init-data
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	type chatInfo struct {
		Title     string `json:"title"`
		Username  string `json:"username"`
		Type      string `json:"type"`
		AvatarURL string `json:"avatar_url"`
	}
	type item struct {
		Name              string             `json:"name"`
		Type              dg.RequirementType `json:"type"`
		Username          string             `json:"username"`
		Status            string             `json:"status"`
		Error             string             `json:"error,omitempty"`
		Link              string             `json:"link,omitempty"`
		ChatInfo          chatInfo           `json:"chat_info"`
		TonMinBalanceNano int64              `json:"ton_min_balance_nano,omitempty"`
		JettonAddress     string             `json:"jetton_address,omitempty"`
		JettonMinAmount   int64              `json:"jetton_min_amount,omitempty"`
		JettonSymbol      string             `json:"jetton_symbol,omitempty"`
		JettonImage       string             `json:"jetton_image,omitempty"`
	}

	results := make([]item, 0, len(g.Requirements))
	allMet := true

	for _, rqm := range g.Requirements {
		it := item{
			Name:              rqm.ChannelTitle,
			Type:              rqm.Type,
			Username:          rqm.ChannelUsername,
			Status:            "failed",
			ChatInfo:          chatInfo{Title: rqm.ChannelTitle, Username: rqm.ChannelUsername, AvatarURL: rqm.AvatarURL},
			TonMinBalanceNano: rqm.TonMinBalanceNano,
			JettonAddress:     rqm.JettonAddress,
			JettonMinAmount:   rqm.JettonMinAmount,
		}
		if rqm.ChannelUsername != "" {
			it.Link = "https://t.me/" + rqm.ChannelUsername
		}
		// Best-effort chat info enrichment (type, avatar/title fallback)
		// Prefer username; fallback to id
		if h.telegram != nil {
			var info *tgsvc.PublicChannelInfo
			if rqm.ChannelUsername != "" {
				if inf, e := h.telegram.GetPublicChannelInfo(c.Context(), rqm.ChannelUsername); e == nil {
					info = inf
				}
			} else if rqm.ChannelID != 0 {
				if inf, e := h.telegram.GetPublicChannelInfoByID(c.Context(), rqm.ChannelID); e == nil {
					info = inf
				}
			}
			if info != nil {
				it.ChatInfo.Type = info.Type
				if it.ChatInfo.Title == "" {
					it.ChatInfo.Title = info.Title
				}
				if it.ChatInfo.Username == "" {
					it.ChatInfo.Username = info.Username
				}
				if it.ChatInfo.AvatarURL == "" {
					it.ChatInfo.AvatarURL = info.AvatarURL
				}
			}
		}

		// Perform requirement check via shared helper
		res := h.checkSingleRequirement(c, userID, &rqm)
		// Map result
		it.Status = res.Status
		it.Error = res.Error
		// Enrich jetton metadata if applicable
		if rqm.Type == dg.RequirementTypeHoldJetton && rqm.JettonAddress != "" {
			if meta, err := h.ton.GetJettonMeta(c.Context(), rqm.JettonAddress); err == nil && meta != nil {
				it.JettonSymbol = meta.Symbol
				it.JettonImage = meta.Image
			}
		}

		results = append(results, it)
		if res.Error != "" || res.Status != "success" {
			allMet = false
		}
	}

	return c.JSON(fiber.Map{
		"giveaway_id": id,
		"results":     results,
		"all_met":     allMet,
	})
}

// checkSingleRequirement verifies one requirement for the given user and returns a minimal result.
func (h *GiveawayHandlersFiber) checkSingleRequirement(c *fiber.Ctx, userID int64, rqm *dg.Requirement) (res struct {
	Status string
	Error  string
}) {
	res.Status = "failed"
	switch rqm.Type {
	case dg.RequirementTypeSubscription:
		chat := ""
		if rqm.ChannelID != 0 {
			chat = fmt.Sprintf("%d", rqm.ChannelID)
		} else if rqm.ChannelUsername != "" {
			chat = "@" + rqm.ChannelUsername
		}
		if chat == "" {
			res.Error = "invalid requirement: no channel"
			return
		}
		ok, e := h.telegram.CheckMembership(c.Context(), userID, chat)
		if e != nil {
			res.Error = e.Error()
			return
		}
		if ok {
			res.Status = "success"
		}
		return
	case dg.RequirementTypeBoost:
		chat := ""
		if rqm.ChannelID != 0 {
			chat = fmt.Sprintf("%d", rqm.ChannelID)
		} else if rqm.ChannelUsername != "" {
			chat = "@" + rqm.ChannelUsername
		}
		if chat == "" {
			res.Error = "invalid requirement: no channel"
			return
		}
		ok, e := h.telegram.CheckBoost(c.Context(), userID, chat)
		if e != nil {
			res.Error = e.Error()
			return
		}
		if ok {
			res.Status = "success"
		}
		return
	case dg.RequirementTypeCustom:
		res.Status = "success"
		return
	case dg.RequirementTypeHoldTON:
		if h.users == nil || h.ton == nil {
			res.Error = "ton service not configured"
			return
		}
		u, err := h.users.GetByID(c.Context(), userID)
		if err != nil || u == nil || u.WalletAddress == "" {
			res.Error = "wallet not linked"
			return
		}
		bal, err := h.ton.GetAddressBalanceNano(c.Context(), u.WalletAddress)
		if err != nil {
			res.Error = err.Error()
			return
		}
		if rqm.TonMinBalanceNano > 0 && bal >= rqm.TonMinBalanceNano {
			res.Status = "success"
		}
		return
	case dg.RequirementTypeHoldJetton:
		if h.users == nil || h.ton == nil {
			res.Error = "ton service not configured"
			return
		}
		u, err := h.users.GetByID(c.Context(), userID)
		if err != nil || u == nil || u.WalletAddress == "" {
			res.Error = "wallet not linked"
			return
		}
		if rqm.JettonAddress == "" || rqm.JettonMinAmount <= 0 {
			res.Error = "invalid jetton requirement"
			return
		}
		bal, err := h.ton.GetJettonBalanceNano(c.Context(), u.WalletAddress, rqm.JettonAddress)
		if err != nil {
			res.Error = err.Error()
			return
		}
		dec, derr := h.ton.GetJettonDecimals(c.Context(), rqm.JettonAddress)
		if derr != nil {
			res.Error = derr.Error()
			return
		}
		req := new(big.Int).SetInt64(rqm.JettonMinAmount)
		pow10 := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dec)), nil)
		req.Mul(req, pow10)
		balBI := new(big.Int).SetInt64(bal)
		if balBI.Cmp(req) >= 0 {
			res.Status = "success"
		}
		return
	default:
		res.Error = "unsupported requirement type"
		return
	}
}
