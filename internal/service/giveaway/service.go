package giveaway

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	dg "github.com/open-builders/giveaway-backend/internal/domain/giveaway"
	repo "github.com/open-builders/giveaway-backend/internal/repository/postgres"
	tg "github.com/open-builders/giveaway-backend/internal/service/telegram"
)

// Service contains business rules for giveaways.
type Service struct {
	repo *repo.GiveawayRepository
	tg   *tg.Client
}

func NewService(r *repo.GiveawayRepository) *Service { return &Service{repo: r} }

// WithTelegram injects a Telegram client for requirements checks and enrichment.
func (s *Service) WithTelegram(client *tg.Client) *Service { s.tg = client; return s }

// Create validates and persists a new giveaway.
func (s *Service) Create(ctx context.Context, g *dg.Giveaway) (string, error) {
	if g == nil {
		return "", errors.New("nil giveaway")
	}
	if g.CreatorID == 0 {
		return "", errors.New("missing creator_id")
	}
	if g.Title == "" {
		return "", errors.New("missing title")
	}
	if g.EndsAt.Before(g.StartedAt) {
		return "", errors.New("ends_at before started_at")
	}
	if g.StartedAt.Before(time.Now().Add(-1 * time.Hour)) {
		return "", errors.New("started_at is too far in the past")
	}
	if g.EndsAt.Sub(g.StartedAt) < 5*time.Minute {
		return "", errors.New("giveaway must last at least 5 minutes")
	}
	if g.MaxWinnersCount <= 0 {
		return "", errors.New("winners_count must be > 0")
	}
	if g.Duration < 0 {
		return "", errors.New("duration must be >= 0")
	}

	id := uuid.NewString()
	g.ID = id
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	g.UpdatedAt = time.Now().UTC()
	if g.Status == "" {
		g.Status = dg.GiveawayStatusScheduled
	}

	g.Status = dg.GiveawayStatusActive

	if err := s.repo.Create(ctx, g); err != nil {
		return "", err
	}
	return id, nil
}

// GetByID fetches giveaway by id.
func (s *Service) GetByID(ctx context.Context, id string) (*dg.Giveaway, error) {
	if id == "" {
		return nil, errors.New("missing id")
	}
	g, err := s.repo.GetByID(ctx, id)
	if err != nil || g == nil {
		return g, err
	}
	// Enrich requirements with channel info via Telegram when possible (best-effort)
	if s.tg != nil {
		for i := range g.Requirements {
			req := &g.Requirements[i]
			if req.Type == dg.RequirementTypeSubscription {
				// Prefer username if present, else resolve from ID by building @username via API
				uname := req.ChannelUsername
				if uname == "" && req.ChannelID != 0 {
					// Telegram API requires @username for avatar URL; we can attempt info via ID not supported reliably
					// Skip if no username
				}
				key := uname
				if key == "" && req.ChannelID != 0 {
					key = fmt.Sprintf("%d", req.ChannelID)
				}
				if key != "" {
					info, err := s.tg.GetPublicChannelInfo(ctx, key)
					if err == nil && info != nil {
						req.ChannelTitle = info.Title
						req.ChannelURL = info.ChannelURL
						req.AvatarURL = info.AvatarURL
						if req.ChannelID == 0 {
							req.ChannelID = info.ID
						}
						if req.ChannelUsername == "" {
							req.ChannelUsername = info.Username
						}
					}
				}
			}
		}
	}
	return g, nil
}

// ListByCreator returns giveaways for the user.
func (s *Service) ListByCreator(ctx context.Context, creatorID int64, limit, offset int) ([]dg.Giveaway, error) {
	if creatorID == 0 {
		return nil, errors.New("missing creator_id")
	}
	return s.repo.ListByCreator(ctx, creatorID, limit, offset)
}

// UpdateStatus changes the status with basic transition validation.
func (s *Service) UpdateStatus(ctx context.Context, id string, status dg.GiveawayStatus) error {
	if id == "" {
		return errors.New("missing id")
	}
	switch status {
	case dg.GiveawayStatusScheduled, dg.GiveawayStatusActive, dg.GiveawayStatusFinished, dg.GiveawayStatusCancelled, "pending":
	default:
		return errors.New("invalid status")
	}
	return s.repo.UpdateStatus(ctx, id, status)
}

// Delete enforces ownership: only creator can delete, atomically.
func (s *Service) Delete(ctx context.Context, id string, requesterID int64) error {
	if id == "" {
		return errors.New("missing id")
	}
	if requesterID == 0 {
		return errors.New("missing requester")
	}
	deleted, err := s.repo.DeleteByOwner(ctx, id, requesterID)
	if err != nil {
		return err
	}
	if deleted {
		return nil
	}
	g, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if g == nil {
		return errors.New("not found")
	}
	return errors.New("forbidden")
}

// Join adds a user to giveaway participants, disallowing self-join (enforced in repo) and returns error if id empty.
func (s *Service) Join(ctx context.Context, id string, userID int64) error {
	if id == "" {
		return errors.New("missing id")
	}
	if userID == 0 {
		return errors.New("missing user_id")
	}
	g, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if g == nil {
		return errors.New("not found")
	}
	if g.CreatorID == userID {
		return errors.New("forbidden")
	}
	if g.Status != dg.GiveawayStatusActive {
		return errors.New("join only allowed for active giveaways")
	}
	// Requirements check (TG errors treated as satisfied)
	if s.tg != nil && len(g.Requirements) > 0 {
		for _, req := range g.Requirements {
			switch req.Type {
			case dg.RequirementTypeSubscription:
				chat := ""
				if req.ChannelID != 0 {
					chat = fmt.Sprintf("%d", req.ChannelID)
				} else if req.ChannelUsername != "" {
					chat = "@" + req.ChannelUsername
				}
				if chat == "" {
					continue
				}
				ok, err := s.tg.CheckMembership(ctx, userID, chat)
				if err != nil {
					continue
				}
				if !ok {
					return errors.New("requirements not satisfied")
				}
			case dg.RequirementTypeBoost:
				chat := ""
				if req.ChannelID != 0 {
					chat = fmt.Sprintf("%d", req.ChannelID)
				} else if req.ChannelUsername != "" {
					chat = "@" + req.ChannelUsername
				}
				if chat == "" {
					continue
				}
				ok, err := s.tg.CheckBoost(ctx, userID, chat)
				if err != nil {
					continue
				}
				if !ok {
					return errors.New("requirements not satisfied")
				}
			}
		}
	}
	return s.repo.Join(ctx, id, userID)
}

// FinishExpired marks all expired giveaways as finished; returns updated count.
func (s *Service) FinishExpired(ctx context.Context) (int64, error) {
	ids, err := s.repo.ListExpiredIDs(ctx)
	if err != nil {
		return 0, err
	}
	var done int64
	for _, id := range ids {
		if err := s.FinishOneWithDistribution(ctx, id); err != nil {
			// Continue on error to not block other giveaways
			continue
		}
		done++
	}
	return done, nil
}

// FinishOneWithDistribution finalizes one giveaway with distribution logic.
func (s *Service) FinishOneWithDistribution(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("missing id")
	}
	g, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if g == nil {
		return errors.New("not found")
	}
	// If custom requirement exists, move to pending and return
	for _, req := range g.Requirements {
		if req.Type == dg.RequirementTypeCustom {
			return s.repo.UpdateStatus(ctx, id, "pending")
		}
	}
	winnersCount := g.MaxWinnersCount
	if winnersCount <= 0 {
		winnersCount = 1
	}
	return s.repo.FinishOneWithDistribution(ctx, id, winnersCount)
}

// FinalizePendingWithCandidates filters provided candidates by non-custom requirements and finalizes giveaway.
func (s *Service) FinalizePendingWithCandidates(ctx context.Context, id string, requesterID int64, candidates []string) (int, int, error) {
	if id == "" {
		return 0, 0, errors.New("missing id")
	}
	if requesterID == 0 {
		return 0, 0, errors.New("unauthorized")
	}
	g, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	if g == nil {
		return 0, 0, errors.New("not found")
	}
	if g.CreatorID != requesterID {
		return 0, 0, errors.New("forbidden")
	}
	if string(g.Status) != "pending" {
		return 0, 0, errors.New("not pending")
	}

	// Parse candidates into numeric IDs, resolve @username using Telegram if needed (best-effort)
	unique := make(map[int64]struct{})
	accepted := 0
	for _, v := range candidates {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		var uid int64
		if strings.HasPrefix(v, "@") {
			if s.tg != nil {
				if info, err := s.tg.GetPublicChannelInfo(ctx, v); err == nil && info != nil {
					// Not a user; skip as we expect user IDs/usernames. In a real impl we might resolve user IDs elsewhere.
					continue
				}
			}
			continue
		} else {
			if idnum, err := strconv.ParseInt(v, 10, 64); err == nil {
				uid = idnum
			} else {
				continue
			}
		}
		if _, ok := unique[uid]; !ok {
			unique[uid] = struct{}{}
			accepted++
		}
	}

	// Filter by non-custom requirements (subscription/boost); TG errors tolerated
	winners := make([]int64, 0, g.MaxWinnersCount)
	for uid := range unique {
		satisfies := true
		if s.tg != nil {
			for _, req := range g.Requirements {
				switch req.Type {
				case dg.RequirementTypeSubscription:
					chat := ""
					if req.ChannelID != 0 {
						chat = fmt.Sprintf("%d", req.ChannelID)
					} else if req.ChannelUsername != "" {
						chat = "@" + req.ChannelUsername
					}
					if chat == "" {
						continue
					}
					ok, err := s.tg.CheckMembership(ctx, uid, chat)
					if err != nil {
						continue
					}
					if !ok {
						satisfies = false
					}
				case dg.RequirementTypeBoost:
					chat := ""
					if req.ChannelID != 0 {
						chat = fmt.Sprintf("%d", req.ChannelID)
					} else if req.ChannelUsername != "" {
						chat = "@" + req.ChannelUsername
					}
					if chat == "" {
						continue
					}
					ok, err := s.tg.CheckBoost(ctx, uid, chat)
					if err != nil {
						continue
					}
					if !ok {
						satisfies = false
					}
				}
				if !satisfies {
					break
				}
			}
		}
		if satisfies {
			winners = append(winners, uid)
		}
	}

	// Trim to winners_count
	if len(winners) > g.MaxWinnersCount {
		winners = winners[:g.MaxWinnersCount]
	}
	if err := s.repo.FinishWithWinners(ctx, id, winners); err != nil {
		return accepted, len(winners), err
	}
	return accepted, len(winners), nil
}

// ListFinishedByCreator returns finished giveaways of a user.
func (s *Service) ListFinishedByCreator(ctx context.Context, creatorID int64, limit, offset int) ([]dg.Giveaway, error) {
	if creatorID == 0 {
		return nil, errors.New("missing creator_id")
	}
	return s.repo.ListFinishedByCreator(ctx, creatorID, limit, offset)
}

// ListActive returns active giveaways with default minParticipants when zero.
func (s *Service) ListActive(ctx context.Context, limit, offset, minParticipants int) ([]dg.Giveaway, error) {
	return s.repo.ListActive(ctx, limit, offset, minParticipants)
}

// GetUserRole returns the role of a given user in a giveaway context.
// owner | winner | participant | user
func (s *Service) GetUserRole(ctx context.Context, g *dg.Giveaway, userID int64) (string, error) {
	if g == nil || userID == 0 {
		return "user", nil
	}
	if g.CreatorID == userID {
		return "owner", nil
	}
	if ok, err := s.repo.IsWinner(ctx, g.ID, userID); err == nil && ok {
		return "winner", nil
	} else if err != nil {
		return "user", err
	}
	if ok, err := s.repo.IsParticipant(ctx, g.ID, userID); err == nil && ok {
		return "participant", nil
	} else if err != nil {
		return "user", err
	}
	return "user", nil
}
