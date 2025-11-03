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
	notify "github.com/open-builders/giveaway-backend/internal/service/notifications"
	tg "github.com/open-builders/giveaway-backend/internal/service/telegram"
	tgutils "github.com/open-builders/giveaway-backend/internal/utils/telegram"
	channelsvc "github.com/open-builders/giveaway-backend/internal/service/channels"
)

// Service contains business rules for giveaways.
type Service struct {
	repo *repo.GiveawayRepository
	tg   *tg.Client
	ntf  *notify.Service
	channels *channelsvc.Service
}

func NewService(r *repo.GiveawayRepository, chs *channelsvc.Service) *Service { return &Service{repo: r, channels: chs} }

// WithTelegram injects a Telegram client for requirements checks and enrichment.
func (s *Service) WithTelegram(client *tg.Client) *Service { s.tg = client; return s }

// WithNotifier injects notifications service for broadcasting updates.
func (s *Service) WithNotifier(n *notify.Service) *Service { s.ntf = n; return s }

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
	// Best-effort notification to creator channels
	if s.ntf != nil {
		go s.ntf.NotifyStarted(context.Background(), g)
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
					// info, err := s.tg.GetPublicChannelInfo(ctx, key)
					// if err == nil && info != nil {
					// 	req.ChannelTitle = info.Title
					// 	req.ChannelURL = info.ChannelURL
					// 	req.AvatarURL = tgutils.BuildAvatarURL(strconv.FormatInt(info.ID, 10))
					// 	if req.ChannelID == 0 {
					// 		req.ChannelID = info.ID
					// 	}
					// 	if req.ChannelUsername == "" {
					// 		req.ChannelUsername = info.Username
					// 	}
					// }

					ch, err := s.channels.GetByID(ctx, req.ChannelID)
					if err == nil && ch != nil {
						req.ChannelTitle = ch.Title
						req.ChannelURL = ch.URL
						req.AvatarURL = ch.AvatarURL
						req.ChannelUsername = ch.Username
						req.ChannelID = ch.ID
					}

					req.AvatarURL = tgutils.BuildAvatarURL(key)
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
	case dg.GiveawayStatusScheduled, dg.GiveawayStatusActive, dg.GiveawayStatusFinished, dg.GiveawayStatusCancelled, dg.GiveawayStatusPending, dg.GiveawayStatusCompleted:
	default:
		return errors.New("invalid status")
	}
	// Allow transition to completed only from pending
	if status == dg.GiveawayStatusCompleted {
		g, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if g == nil {
			return errors.New("not found")
		}
		if g.Status != dg.GiveawayStatusPending {
			return errors.New("transition not allowed")
		}
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
	// If custom requirement exists, move to pending and return (winners will be uploaded manually)
	for _, req := range g.Requirements {
		if req.Type == dg.RequirementTypeCustom {
			return s.repo.UpdateStatus(ctx, id, dg.GiveawayStatusPending)
		}
	}
	winnersCount := g.MaxWinnersCount
	if winnersCount <= 0 {
		winnersCount = 1
	}
	if err := s.repo.FinishOneWithDistribution(ctx, id, winnersCount); err != nil {
		return err
	}
	// Best-effort completion notification
	if s.ntf != nil {
		go s.ntf.NotifyCompleted(context.Background(), g, winnersCount)
	}
	return nil
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

	// Parse candidates into numeric IDs, ignore @usernames here (we require id),
	// then keep only those who are participants of the giveaway (ensures user exists in DB)
	unique := make(map[int64]struct{})
	for _, v := range candidates {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "@") {
			// usernames are not accepted for finalization here
			continue
		}
		if idnum, err := strconv.ParseInt(v, 10, 64); err == nil {
			unique[idnum] = struct{}{}
		}
	}

	// Filter by participation to avoid foreign key violations
	filtered := make([]int64, 0, len(unique))
	for uid := range unique {
		ok, err := s.repo.IsParticipant(ctx, id, uid)
		if err != nil {
			// ignore repo error for one uid and skip this candidate
			continue
		}
		if ok {
			filtered = append(filtered, uid)
		}
	}
	accepted := len(filtered)

	// Filter by non-custom requirements (subscription/boost); TG errors tolerated
	winners := make([]int64, 0, g.MaxWinnersCount)
	for _, uid := range filtered {
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

// FinalizeWithWinners finalizes a pending giveaway with the provided winners list (ordered by place),
// validates ownership, status, and participation, and distributes prizes according to quantities.
func (s *Service) FinalizeWithWinners(ctx context.Context, id string, winners []int64) error {
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
	// Only creator can finalize
	// Caller context should ensure auth; we infer requester from business flow is creator
	// For stricter checks, this method could accept requesterID; keeping simple here.
	// Enforce pending status for manual finalization
	if string(g.Status) != "pending" {
		return errors.New("not pending")
	}
	if len(winners) == 0 {
		return errors.New("no winners")
	}
	// Keep only participants
	filtered := make([]int64, 0, len(winners))
	seen := make(map[int64]struct{}, len(winners))
	for _, uid := range winners {
		if uid == 0 {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		ok, err := s.repo.IsParticipant(ctx, id, uid)
		if err != nil || !ok {
			continue
		}
		filtered = append(filtered, uid)
	}
	if len(filtered) == 0 {
		return errors.New("no valid winners")
	}
	// Trim to winners_count
	max := g.MaxWinnersCount
	if max > 0 && len(filtered) > max {
		filtered = filtered[:max]
	}
	return s.repo.FinishWithWinners(ctx, id, filtered)
}

// SetManualWinners stores winners and distributes prizes while keeping giveaway pending.
func (s *Service) SetManualWinners(ctx context.Context, id string, requesterID int64, winners []int64) error {
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
	if g.CreatorID != requesterID {
		return errors.New("forbidden")
	}
	if string(g.Status) != "pending" {
		return errors.New("not pending")
	}
	if len(winners) == 0 {
		return errors.New("no winners")
	}
	// Keep only participants, dedupe
	filtered := make([]int64, 0, len(winners))
	seen := make(map[int64]struct{}, len(winners))
	for _, uid := range winners {
		if uid == 0 {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		ok, err := s.repo.IsParticipant(ctx, id, uid)
		if err != nil || !ok {
			continue
		}
		filtered = append(filtered, uid)
	}
	if len(filtered) == 0 {
		return errors.New("no valid winners")
	}
	max := g.MaxWinnersCount
	if max > 0 && len(filtered) > max {
		filtered = filtered[:max]
	}
	return s.repo.SetManualWinners(ctx, id, filtered)
}

// ListWinnersWithPrizes proxies repository to fetch winners and their prizes.
func (s *Service) ListWinnersWithPrizes(ctx context.Context, id string) ([]dg.Winner, error) {
	if id == "" {
		return nil, errors.New("missing id")
	}
	return s.repo.ListWinnersWithPrizes(ctx, id)
}

// ClearManualWinners removes all winners for a pending giveaway; only creator can perform.
func (s *Service) ClearManualWinners(ctx context.Context, id string, requesterID int64) error {
	if id == "" {
		return errors.New("missing id")
	}
	if requesterID == 0 {
		return errors.New("unauthorized")
	}
	g, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if g == nil {
		return errors.New("not found")
	}
	if g.CreatorID != requesterID {
		return errors.New("forbidden")
	}
	if g.Status != dg.GiveawayStatusPending {
		return errors.New("not pending")
	}
	return s.repo.ClearWinners(ctx, id)
}
