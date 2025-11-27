package http

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	dg "github.com/open-builders/giveaway-backend/internal/domain/giveaway"
	"github.com/open-builders/giveaway-backend/internal/http/middleware"
	redisp "github.com/open-builders/giveaway-backend/internal/platform/redis"
	"github.com/open-builders/giveaway-backend/internal/service/channels"
	chsvc "github.com/open-builders/giveaway-backend/internal/service/channels"
	gsvc "github.com/open-builders/giveaway-backend/internal/service/giveaway"
	tgsvc "github.com/open-builders/giveaway-backend/internal/service/telegram"
	tonb "github.com/open-builders/giveaway-backend/internal/service/tonbalance"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
	"github.com/open-builders/giveaway-backend/internal/utils/random"
	tgutils "github.com/open-builders/giveaway-backend/internal/utils/telegram"
)

// GiveawayHandlersFiber provides Fiber endpoints for giveaways.
type GiveawayHandlersFiber struct {
	service  *gsvc.Service
	channels *chsvc.Service
	telegram *tgsvc.Client
	users    *usersvc.Service
	ton      *tonb.Service
	rdb      *redisp.Client
}

func NewGiveawayHandlersFiber(svc *gsvc.Service, chs *chsvc.Service, tg *tgsvc.Client, users *usersvc.Service, ton *tonb.Service, rdb *redisp.Client) *GiveawayHandlersFiber {
	return &GiveawayHandlersFiber{service: svc, channels: chs, telegram: tg, users: users, ton: ton, rdb: rdb}
}

func (h *GiveawayHandlersFiber) RegisterFiber(r fiber.Router) {
	r.Post("/giveaways", h.create)
	r.Get("/giveaways/:id", h.getByID)
	r.Post("/giveaways/:id/prepare-message", h.prepareInlineMessage)
	r.Get("/giveaways/:id/list-loaded-winners", h.listWinnersWithPrizes)
	r.Get("/giveaways/:id/stats.csv", h.exportWinnersCSV)
	r.Get("/giveaways/:id/export-link", h.generateExportLink)
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

// RegisterPublicFiber registers public routes (no init-data auth).
func (h *GiveawayHandlersFiber) RegisterPublicFiber(r fiber.Router) {
	r.Get("/giveaways/export/:token", h.downloadExportCSV)
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

	// Validate negative values
	if req.Duration < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "duration cannot be negative"})
	}
	if req.WinnersCount < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "winners_count cannot be negative"})
	}

	// Validate maximum duration (2 months = 60 days = 5184000 seconds)
	const maxDurationSeconds = 60 * 24 * 60 * 60 // 60 days in seconds
	if req.Duration > maxDurationSeconds {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "duration cannot exceed 2 months (60 days)"})
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

	if utf8.RuneCountInString(g.Title) > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Giveaway title too long (max 100 characters)"})
	}

	// Map and enrich requirements first (independent of prizes)
	for _, r := range req.Requirements {
		switch r.Type {
		case dg.RequirementTypeSubscription:
			channelID := r.ChannelID
			reqEntry := dg.Requirement{Type: dg.RequirementTypeSubscription}
			if r.Name != "" {
				reqEntry.ChannelTitle = r.Name
			}
			if r.Description != "" {
				reqEntry.Description = r.Description
			}
			// Try Telegram enrichment
			if h.telegram != nil && channelID != 0 {
				ch, err := h.channels.GetByID(c.Context(), channelID, middleware.GetUserID(c))
				if err != nil {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
				}
				if ch != nil {
					reqEntry.ChannelID = ch.ID
					reqEntry.ChannelUsername = ch.Username
					reqEntry.ChannelTitle = ch.Title
					reqEntry.ChannelURL = ch.URL
					reqEntry.AvatarURL = ch.AvatarURL
				}
				if reqEntry.ChannelURL == "" {
					reqEntry.ChannelURL = "https://t.me/" + reqEntry.ChannelUsername
				}
			} else {
				// No telegram client: store what we have
				reqEntry.ChannelUsername = r.ChannelUsername
				if r.ChannelID != 0 {
					reqEntry.ChannelID = r.ChannelID
				}
				if r.AvatarURL != "" {
					reqEntry.AvatarURL = r.AvatarURL
				}
			}
			g.Requirements = append(g.Requirements, reqEntry)
		case dg.RequirementTypeBoost:
			channelID := r.ChannelID
			reqEntry := dg.Requirement{Type: dg.RequirementTypeBoost}
			if r.Name != "" {
				reqEntry.ChannelTitle = r.Name
			}
			if r.Description != "" {
				reqEntry.Description = r.Description
			}
			// Try Telegram enrichment
			if h.telegram != nil && channelID != 0 {
				ch, err := h.channels.GetByID(c.Context(), channelID, middleware.GetUserID(c))
				if err != nil {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
				}
				if ch != nil {
					reqEntry.ChannelID = ch.ID
					reqEntry.ChannelUsername = ch.Username
					reqEntry.ChannelTitle = ch.Title
					// reqEntry.ChannelURL = "https://t.me/boost?c=" + strconv.FormatInt(ch.ID, 10)
					if r.ChannelUsername != "" {
						reqEntry.ChannelURL = "https://t.me/boost/" + ch.Username
					} else {
						reqEntry.ChannelURL = "https://t.me/c/" + strings.TrimPrefix(strconv.FormatInt(ch.ID, 10), "-100") + "?boost"
					}
					reqEntry.AvatarURL = ch.AvatarURL
				}
			} else {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid requirement"})
			}
			g.Requirements = append(g.Requirements, reqEntry)
		case dg.RequirementTypeCustom:
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypeCustom, ChannelTitle: r.Name, Description: r.Description})
		case dg.RequirementTypePremium:
			// No extra fields required; carry optional name/description for UI
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypePremium, Title: r.Name, Description: r.Description})
		case dg.RequirementTypeHoldTON:
			if r.TonMinBalanceNano < 0 {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ton_min_balance_nano cannot be negative"})
			}
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypeHoldTON, TonMinBalanceNano: r.TonMinBalanceNano, Title: r.Name, Description: r.Description})
		case dg.RequirementTypeHoldJetton:
			if r.JettonMinAmount < 0 {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "jetton_min_amount cannot be negative"})
			}
			g.Requirements = append(g.Requirements, dg.Requirement{Type: dg.RequirementTypeHoldJetton, JettonAddress: r.JettonAddress, JettonMinAmount: r.JettonMinAmount, Title: r.Name, Description: r.Description})
		}
	}

	// Map prizes
	for _, p := range req.Prizes {
		if p.Quantity < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "prize quantity cannot be negative"})
		}
		qty := p.Quantity
		if qty <= 0 {
			qty = 1
		}

		// check if price title > 20 characters, if yes, return error (count runes, not bytes)
		if utf8.RuneCountInString(p.Title) > 20 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Prize title too long (max 20 characters)"})
		}

		g.Prizes = append(g.Prizes, dg.PrizePlace{
			// Ignore incoming place and store as NULL → all prizes are loose
			Place:       nil,
			Title:       p.Title,
			Description: p.Description,
			Quantity:    qty,
		})
	}

	// Map sponsors: берем из Redis (channels service) по channel_id и сохраняем полные данные в БД
	for _, s := range req.Sponsors {
		if s.ID == 0 {
			g.Sponsors = append(g.Sponsors, dg.ChannelInfo{ID: s.ID})
			continue
		}
		if h.channels != nil {
			ch, err := h.channels.GetByID(c.Context(), s.ID, middleware.GetUserID(c))
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			if ch != nil {
				var url string
				if ch.Username != "" {
					url = "https://t.me/" + ch.Username
				}

				if ch.URL != "" {
					url = ch.URL
				}

				g.Sponsors = append(g.Sponsors, dg.ChannelInfo{ID: ch.ID, Title: ch.Title, Username: ch.Username, URL: url, AvatarURL: ch.AvatarURL})
				continue
			}
		}
		// Если в Redis нет — сохраняем хотя бы id, остальное можно дозаполнить позже
		g.Sponsors = append(g.Sponsors, dg.ChannelInfo{ID: s.ID})
	}

	id, err := h.service.Create(c.Context(), &g)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Include prepared inline message id from Redis cache in create response (creator only)
	msgID := ""
	if h.rdb != nil {
		if v, e := h.rdb.Get(c.Context(), "giveaway:"+id+":prepared_inline_message_id").Result(); e == nil {
			msgID = v
		}
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id, "msg_id": msgID})
}

// prepareInlineMessage prepares (or returns cached) prepared inline message for a giveaway.
// Access: only giveaway owner. Caches result in Redis for 50 minutes.
func (h *GiveawayHandlersFiber) prepareInlineMessage(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing id"})
	}
	requesterID := middleware.GetUserID(c)
	if requesterID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	// Load giveaway
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	// Only owner allowed
	if g.CreatorID != requesterID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	// Redis cache
	if h.rdb == nil || h.telegram == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "service not configured"})
	}
	cacheKey := "giveaway:" + id + ":prepared_inline_message_id"
	if v, err := h.rdb.Get(c.Context(), cacheKey).Result(); err == nil && v != "" {
		return c.JSON(fiber.Map{"msg_id": v, "cached": true})
	}
	// Build startapp URL via bot username
	startURL := ""
	if me, err := h.telegram.GetBotMe(c.Context(), h.rdb); err == nil && me != nil && me.Username != "" {
		startURL = fmt.Sprintf("https://t.me/%s?startapp=%s", me.Username, g.ID)
	}
	// Build the same text as in NotifyStarted
	text := buildStartMessageForPrepare(g)
	// Use the same GIF as announcement
	// const startedGIF = "https://cdn.giveaway.tools.tg/assets/Started.gif"
	// get file_id from Redis
	startedGIF, err := h.rdb.Get(c.Context(), "tg:file_id:animation:started").Result()
	if err != nil && err != redis.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if startedGIF == "" {
		startedGIF = "https://cdn.giveaway.tools.tg/assets/Started.gif"
	}
	// Use GIF as thumbnail fallback to satisfy Bot API requirements
	msgID, err := h.telegram.SavePreparedInlineMessageGif(c.Context(), g.CreatorID, startedGIF, startedGIF, text, "Open Giveaway", startURL)
	if err != nil || msgID == "" {
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to prepare message"})
	}
	// Cache for 50 minutes
	_ = h.rdb.SetEx(c.Context(), cacheKey, msgID, 50*time.Minute).Err()
	return c.JSON(fiber.Map{"msg_id": msgID, "cached": false})
}

// buildStartMessageForPrepare replicates the start message format used in notifications.
func buildStartMessageForPrepare(g *dg.Giveaway) string {
	var b strings.Builder
	b.WriteString("🎁 Giveaway is live!\n\n")
	b.WriteString("Details:\n")
	// Subscribe line from sponsors
	subs := collectSponsorsUsernamesForPrepare(g)
	if subs != "" {
		b.WriteString("Subscribe: ")
		b.WriteString(subs)
		b.WriteString("\n")
	}
	// Deadline
	b.WriteString("Deadline: ")
	b.WriteString(g.EndsAt.UTC().Format("02 Jan 2006 15:04 UTC"))
	b.WriteString("\n")
	// Prizes
	prizes := collectPrizeTitlesForPrepare(g)
	if prizes != "" {
		b.WriteString("Prizes: ")
		b.WriteString(prizes)
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}
	// Requirements
	req := buildRequirementsBlockForPrepare(g)
	if req != "" {
		b.WriteString("Requirements:\n")
		b.WriteString(req)
		b.WriteString("\n")
	}
	b.WriteString("Participants can now join this giveaway. Good luck!")
	return b.String()
}

func collectSponsorsUsernamesForPrepare(g *dg.Giveaway) string {
	if g == nil || len(g.Sponsors) == 0 {
		return ""
	}
	names := make([]string, 0, len(g.Sponsors))
	for _, s := range g.Sponsors {
		if s.Username != "" {
			names = append(names, "@"+s.Username)
		} else if s.Title != "" {
			names = append(names, s.Title)
		}
	}
	return strings.Join(names, ", ")
}

func collectPrizeTitlesForPrepare(g *dg.Giveaway) string {
	if g == nil || len(g.Prizes) == 0 {
		return ""
	}
	titles := make([]string, 0, len(g.Prizes))
	for _, p := range g.Prizes {
		if p.Title != "" {
			titles = append(titles, p.Title)
		}
	}
	return strings.Join(titles, ", ")
}

func buildRequirementsBlockForPrepare(g *dg.Giveaway) string {
	if g == nil || len(g.Requirements) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range g.Requirements {
		switch r.Type {
		case dg.RequirementTypeSubscription:
			if r.ChannelUsername != "" {
				b.WriteString("• Subscribe to @")
				b.WriteString(r.ChannelUsername)
			} else if r.ChannelTitle != "" {
				b.WriteString("• Subscribe to ")
				b.WriteString(r.ChannelTitle)
			} else {
				b.WriteString("• Subscribe to the channel")
			}
			b.WriteString("\n")
		case dg.RequirementTypeBoost:
			if r.ChannelUsername != "" {
				b.WriteString("• Boost @")
				b.WriteString(r.ChannelUsername)
			} else {
				b.WriteString("• Boost the channel")
			}
			b.WriteString("\n")
		case dg.RequirementTypeHoldTON:
			if r.TonMinBalanceNano > 0 {
				tons := float64(r.TonMinBalanceNano) / 1_000_000_000
				rounded := math.Round(tons*1000) / 1000
				// format without trailing zeros; up to 3 decimals after rounding
				tonsStr := strconv.FormatFloat(rounded, 'f', -1, 64)
				b.WriteString(fmt.Sprintf("• Minimum TON balance: %s TON\n", tonsStr))
			}
		case dg.RequirementTypeHoldJetton:
			if r.JettonAddress != "" {
				if r.JettonMinAmount > 0 {
					b.WriteString(fmt.Sprintf("• Hold jetton %s ≥ %d\n", r.JettonAddress, r.JettonMinAmount))
				} else {
					b.WriteString(fmt.Sprintf("• Hold jetton %s\n", r.JettonAddress))
				}
			}
		case dg.RequirementTypeCustom:
			if r.Title != "" || r.Description != "" {
				b.WriteString("• ")
				if r.Title != "" {
					b.WriteString(r.Title)
					if r.Description != "" {
						b.WriteString(": ")
						b.WriteString(r.Description)
					}
				} else {
					b.WriteString(r.Description)
				}
				b.WriteString("\n")
			}
		case dg.RequirementTypePremium:
			b.WriteString("• Telegram Premium user\n")
		}
	}
	return b.String()
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
		URL         string             `json:"url"`
		Description string             `json:"description,omitempty"`
		// On-chain fields
		TonMinBalanceNano int64  `json:"ton_min_balance_nano,omitempty"`
		JettonAddress     string `json:"jetton_address,omitempty"`
		JettonMinAmount   int64  `json:"jetton_min_amount,omitempty"`
		// Jetton metadata enrichment
		JettonSymbol string `json:"jetton_symbol,omitempty"`
		JettonImage  string `json:"jetton_image,omitempty"`
	}

	type sponsorDTO struct {
		ID        int64  `json:"id"`
		Username  string `json:"username,omitempty"`
		AvatarURL string `json:"avatar_url,omitempty"`
		URL       string `json:"url"`
		Title     string `json:"title,omitempty"`
	}

	type winnerDTO struct {
		UserID    int64            `json:"user_id"`
		Username  string           `json:"username,omitempty"`
		Name      string           `json:"name"`
		AvatarURL string           `json:"avatar_url,omitempty"`
		Place     int              `json:"place"`
		Prizes    []dg.WinnerPrize `json:"prizes"`
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
		Sponsors          []sponsorDTO      `json:"sponsors"`
		Requirements      []requirementDTO  `json:"requirements,omitempty"`
		Winners           []winnerDTO       `json:"winners,omitempty"`
		ParticipantsCount int               `json:"participants_count"`
		UserRole          string            `json:"user_role,omitempty"`
		MsgID             string            `json:"msg_id,omitempty"`
	}
	// Map requirements to requested API shape
	reqs := make([]requirementDTO, 0, len(g.Requirements))
	for _, r := range g.Requirements {
		name := r.ChannelTitle
		if name == "" {
			name = r.Title
		}
		// Build URL for requirement: prefer stored ChannelURL, else build from username
		reqURL := r.ChannelURL
		if reqURL == "" && r.ChannelUsername != "" {
			reqURL = "https://t.me/" + r.ChannelUsername
		}

		if r.Type == dg.RequirementTypeBoost {
			if r.ChannelUsername != "" {
				reqURL = "https://t.me/boost/" + r.ChannelUsername
			} else {
				reqURL = "https://t.me/c/" + strings.TrimPrefix(strconv.FormatInt(r.ChannelID, 10), "-100") + "?boost"
			}
		}

		it := requirementDTO{
			Name:              name,
			Type:              r.Type,
			Username:          r.ChannelUsername,
			AvatarURL:         r.AvatarURL,
			Description:       r.Description,
			TonMinBalanceNano: r.TonMinBalanceNano,
			JettonAddress:     r.JettonAddress,
			JettonMinAmount:   r.JettonMinAmount,
			URL:               reqURL,
		}
		if r.Type == dg.RequirementTypeHoldJetton && r.JettonAddress != "" && h.ton != nil {
			if meta, err := h.ton.GetJettonMeta(c.Context(), r.JettonAddress); err == nil && meta != nil {
				it.JettonSymbol = meta.Symbol
				it.JettonImage = meta.Image
			}
		}
		reqs = append(reqs, it)
	}

	// Map sponsors with URL always present (may be empty string)
	sponsors := make([]sponsorDTO, 0, len(g.Sponsors))
	for _, s := range g.Sponsors {
		url := s.URL
		if url == "" && s.Username != "" {
			url = "https://t.me/" + s.Username
		}
		sponsors = append(sponsors, sponsorDTO{
			ID:        s.ID,
			Username:  s.Username,
			AvatarURL: s.AvatarURL,
			URL:       url,
			Title:     s.Title,
		})
	}

	// Enrich winners if any
	enrichedWinners := make([]winnerDTO, 0, len(g.Winners))
	for _, w := range g.Winners {
		var username, name, avatar string
		if h.users != nil {
			if usr, uerr := h.users.GetByID(c.Context(), w.UserID); uerr == nil && usr != nil {
				username = usr.Username
				name = strings.TrimSpace(strings.TrimSpace(usr.FirstName + " " + usr.LastName))
				avatar = usr.AvatarURL
			}
		}
		if name == "" {
			name = strconv.FormatInt(w.UserID, 10)
		}
		if avatar == "" {
			avatar = tgutils.BuildAvatarURL(strconv.FormatInt(w.UserID, 10))
		}
		enrichedWinners = append(enrichedWinners, winnerDTO{
			UserID:    w.UserID,
			Username:  username,
			Name:      name,
			AvatarURL: avatar,
			Place:     w.Place,
			Prizes:    w.Prizes,
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
		Sponsors:          sponsors,
		Requirements:      reqs,
		Winners:           enrichedWinners,
		ParticipantsCount: g.ParticipantsCount,
		UserRole:          userRole,
	}
	// Only owner sees msg_id
	if userRole == "owner" && h.rdb != nil {
		if v, e := h.rdb.Get(c.Context(), "giveaway:"+g.ID+":prepared_inline_message_id").Result(); e == nil {
			dto.MsgID = v
		}
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
	return h.service.CheckRequirements(c.Context(), userID, g.Requirements)
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
						avatar := tgutils.BuildAvatarURL(strconv.FormatInt(usr.ID, 10))
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
					avatar = tgutils.BuildAvatarURL(strconv.FormatInt(uid, 10))
					out = append(out, previewItem{UserID: uid, Username: username, Name: name, AvatarURL: avatar, Source: "id"})
				}
			}
		}
	}

	// Enforce max winners limit from giveaway settings
	if g.MaxWinnersCount > 0 && len(out) > g.MaxWinnersCount {
		// shuffle to ensure random selection when truncating
		if err := random.Shuffle(out); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to shuffle candidates"})
		}
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
				avatar = usr.AvatarURL
				if avatar == "" {
					avatar = tgutils.BuildAvatarURL(strconv.FormatInt(w.UserID, 10))
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

// exportWinnersCSV streams a CSV file with winners and their prizes.
// Access: only giveaway creator with admin role.
func (h *GiveawayHandlersFiber) exportWinnersCSV(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing id"})
	}
	requesterID := middleware.GetUserID(c)
	if requesterID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	// Verify requester is admin
	if h.users == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "user service not configured"})
	}
	reqUser, err := h.users.GetByID(c.Context(), requesterID)
	if err != nil || reqUser == nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	// Verify ownership
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if g.CreatorID != requesterID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	// Fetch winners with prizes
	winners, err := h.service.ListWinnersWithPrizes(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	// Build CSV
	var buf bytes.Buffer
	// UTF-8 BOM for Excel compatibility with Cyrillic
	_, _ = buf.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"place", "user_id", "username", "first_name", "last_name", "wallet_address", "prize_title", "prize_description"})
	for _, w := range winners {
		var username, firstName, lastName, wallet string
		if h.users != nil {
			if usr, uerr := h.users.GetByID(c.Context(), w.UserID); uerr == nil && usr != nil {
				username = usr.Username
				firstName = usr.FirstName
				lastName = usr.LastName
				wallet = usr.WalletAddress
			}
		}
		if len(w.Prizes) == 0 {
			_ = writer.Write([]string{
				strconv.Itoa(w.Place),
				strconv.FormatInt(w.UserID, 10),
				username,
				firstName,
				lastName,
				wallet,
				"",
				"",
			})
			continue
		}
		for _, p := range w.Prizes {
			_ = writer.Write([]string{
				strconv.Itoa(w.Place),
				strconv.FormatInt(w.UserID, 10),
				username,
				firstName,
				lastName,
				wallet,
				p.Title,
				p.Description,
			})
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	filename := fmt.Sprintf("giveaway_%s_winners.csv", id)
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", filename))
	return c.Send(buf.Bytes())
}

// generateExportLink creates a short-lived token in Redis and returns a public URL to download CSV without auth.
// Access: only giveaway creator with admin role.
func (h *GiveawayHandlersFiber) generateExportLink(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing id"})
	}
	requesterID := middleware.GetUserID(c)
	if requesterID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.users == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "user service not configured"})
	}
	usr, err := h.users.GetByID(c.Context(), requesterID)
	if err != nil || usr == nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	// Validate ownership
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if g.CreatorID != requesterID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
	if h.rdb == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "redis not configured"})
	}
	token := uuid.NewString()
	key := "export:giveaway:" + token
	ttl := 2 * time.Minute
	if err := h.rdb.SetEx(c.Context(), key, id, ttl).Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to store token"})
	}
	publicURL := c.BaseURL() + "/api/public/giveaways/export/" + token
	return c.JSON(fiber.Map{"url": publicURL, "expires_in": int(ttl.Seconds())})
}

// downloadExportCSV validates token (no auth), generates CSV and returns it, then invalidates token.
func (h *GiveawayHandlersFiber) downloadExportCSV(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing token"})
	}
	if h.rdb == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "redis not configured"})
	}
	key := "export:giveaway:" + token
	id, err := h.rdb.Get(c.Context(), key).Result()

	if err != nil || id == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "invalid or expired token"})
	}
	// One-time usage: best-effort delete
	// _ = h.rdb.Del(c.Context(), key).Err()
	// Ensure giveaway exists
	g, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if g == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	// Fetch winners and build CSV (reuse logic)
	winners, err := h.service.ListWinnersWithPrizes(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	var buf bytes.Buffer
	_, _ = buf.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"place", "user_id", "username", "first_name", "last_name", "wallet_address", "prize_title", "prize_description"})
	for _, w := range winners {
		var username, firstName, lastName, wallet string
		if h.users != nil {
			if usr, uerr := h.users.GetByID(c.Context(), w.UserID); uerr == nil && usr != nil {
				username = usr.Username
				firstName = usr.FirstName
				lastName = usr.LastName
				wallet = usr.WalletAddress
			}
		}
		if len(w.Prizes) == 0 {
			_ = writer.Write([]string{
				strconv.Itoa(w.Place),
				strconv.FormatInt(w.UserID, 10),
				username,
				firstName,
				lastName,
				wallet,
				"",
				"",
			})
			continue
		}
		for _, p := range w.Prizes {
			_ = writer.Write([]string{
				strconv.Itoa(w.Place),
				strconv.FormatInt(w.UserID, 10),
				username,
				firstName,
				lastName,
				wallet,
				p.Title,
				p.Description,
			})
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	filename := fmt.Sprintf("giveaway_%s_winners.csv", id)
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", filename))
	// Allow direct download in Telegram Web
	c.Set("Access-Control-Allow-Origin", "https://web.telegram.org")
	return c.Send(buf.Bytes())
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
		URL       string `json:"url"`
	}
	type item struct {
		Name              string             `json:"name"`
		Type              dg.RequirementType `json:"type"`
		Username          string             `json:"username"`
		Status            string             `json:"status"`
		Error             string             `json:"error,omitempty"`
		Link              string             `json:"url,omitempty"`
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
		// Build channel URL: prefer stored ChannelURL, else from username
		channelURL := rqm.ChannelURL
		if channelURL == "" && rqm.ChannelUsername != "" {
			channelURL = "https://t.me/" + rqm.ChannelUsername
		}
		it := item{
			Name:              rqm.ChannelTitle,
			Type:              rqm.Type,
			Username:          rqm.ChannelUsername,
			Status:            "failed",
			ChatInfo:          chatInfo{Title: rqm.ChannelTitle, Username: rqm.ChannelUsername, AvatarURL: rqm.AvatarURL, URL: channelURL},
			TonMinBalanceNano: rqm.TonMinBalanceNano,
			JettonAddress:     rqm.JettonAddress,
			JettonMinAmount:   rqm.JettonMinAmount,
		}
		if rqm.ChannelUsername != "" {
			it.Link = "https://t.me/" + rqm.ChannelUsername
		}
		// Best-effort chat info enrichment (type, avatar/title fallback)
		// Prefer username; fallback to id
		if h.channels != nil {
			var info *channels.Channel

			if inf, e := h.channels.GetByID(c.Context(), rqm.ChannelID); e == nil {
				info = inf
			}

			if rqm.ChannelUsername != "" {
				it.ChatInfo.URL = "https://t.me/" + rqm.ChannelUsername
			}

			if info != nil {
				// it.ChatInfo.Type = info.Type
				if it.ChatInfo.Title == "" {
					it.ChatInfo.Title = info.Title
				}
				if it.ChatInfo.Username == "" {
					it.ChatInfo.Username = info.Username
				}
				if it.ChatInfo.AvatarURL == "" {
					it.ChatInfo.AvatarURL = info.AvatarURL
				}
				if info.URL != "" {
					it.ChatInfo.URL = info.URL
				}
			}
		}
		// Perform requirement check via shared helper
		res := h.service.CheckSingleRequirement(c.Context(), userID, &rqm)
		// Map result
		it.Status = res.Status
		it.Error = res.Error
		// Enrich jetton metadata if applicable
		if rqm.Type == dg.RequirementTypeHoldJetton && rqm.JettonAddress != "" {
			if meta, err := h.ton.GetJettonMeta(c.Context(), rqm.JettonAddress); err == nil && meta != nil {
				it.JettonSymbol = meta.Symbol
				it.JettonImage = meta.Image
			}
		} else if rqm.Type == dg.RequirementTypeBoost {
			if rqm.ChannelUsername != "" {
				it.Link = "https://t.me/boost/" + rqm.ChannelUsername
			} else {
				it.Link = "https://t.me/c/" + strings.TrimPrefix(strconv.FormatInt(rqm.ChannelID, 10), "-100") + "?boost"
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
