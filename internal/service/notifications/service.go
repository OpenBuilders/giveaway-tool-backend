package notifications

import (
	"context"
	"fmt"
	"strings"
	"time"

	dg "github.com/open-builders/giveaway-backend/internal/domain/giveaway"
	redisp "github.com/open-builders/giveaway-backend/internal/platform/redis"
	"github.com/open-builders/giveaway-backend/internal/service/channels"
	tg "github.com/open-builders/giveaway-backend/internal/service/telegram"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
)

// Service formats and sends giveaway notifications to creator channels.
type Service struct {
	tg         *tg.Client
	channels   *channels.Service
	webAppBase string
	rdb        *redisp.Client
	users      *usersvc.Service
}

func NewService(tgc *tg.Client, chs *channels.Service, webAppBaseURL string, rdb *redisp.Client, users *usersvc.Service) *Service {
	return &Service{tg: tgc, channels: chs, webAppBase: strings.TrimRight(webAppBaseURL, "/"), rdb: rdb, users: users}
}

// NotifyStarted posts an announcement to all creator channels when a giveaway starts.
func (s *Service) NotifyStarted(ctx context.Context, g *dg.Giveaway) {
	if s == nil || s.tg == nil || s.channels == nil || g == nil || g.CreatorID == 0 {
		return
	}
	// Build message
	text := buildStartMessage(g)
	animationID := s.tg.Media["giveaway_started"]

	// Button URL: link to current bot username
	btnURL := ""
	if s.rdb != nil {
		if me, err := s.tg.GetBotMe(ctx, s.rdb); err == nil && me != nil && me.Username != "" {
			btnURL = fmt.Sprintf("https://t.me/%s?startapp=%s", me.Username, g.ID)
		}
	}
	// Deliver to each creator channel
	chs := g.Sponsors
	for _, ch := range chs {
		if ch.ID == 0 {
			continue
		}
		_ = s.tg.SendAnimation(ctx, ch.ID, animationID, text, "HTML", "Open Giveaway", btnURL)
	}
}

// NotifyCompleted posts results to all creator channels when a giveaway completes.
func (s *Service) NotifyCompleted(ctx context.Context, g *dg.Giveaway, winnersSelected int) {
	if s == nil || s.tg == nil || s.channels == nil || g == nil || g.CreatorID == 0 {
		return
	}
	text := buildCompletedMessage(g, winnersSelected)
	animationID := s.tg.Media["giveaway_finished"]
	
	btnURL := s.buildWebAppURL(g.ID)
	// Send to sponsor channels
	for _, ch := range g.Sponsors {
		if ch.ID == 0 {
			continue
		}
		_ = s.tg.SendAnimation(ctx, ch.ID, animationID, text, "HTML", "View Results", btnURL)
	}
}

func (s *Service) buildWebAppURL(id string) string {
	if s.webAppBase == "" {
		return ""
	}
	return fmt.Sprintf("%s/g/%s", s.webAppBase, id)
}

func (s *Service) buildStartAppURL(id string) string {
	if s.tg == nil || s.rdb == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	me, err := s.tg.GetBotMe(ctx, s.rdb)
	if err != nil || me == nil || me.Username == "" {
		return ""
	}
	return fmt.Sprintf("https://t.me/%s?startapp=%s", me.Username, id)
}

// NotifyPending announces that winners will be selected manually (pending state).
func (s *Service) NotifyPending(ctx context.Context, g *dg.Giveaway) {
	if s == nil || s.tg == nil || g == nil {
		return
	}
	text := fmt.Sprintf("⏳ Giveaway “%s” is now pending.\nOwners are selecting winners manually. Results will be announced soon.", g.Title)
	btnURL := s.buildStartAppURL(g.ID)
	for _, ch := range g.Sponsors {
		if ch.ID == 0 {
			continue
		}
		_ = s.tg.SendMessage(ctx, ch.ID, text, "HTML", "Open Giveaway", btnURL, true)
	}
}

// NotifyWinnersSelected announces winners in sponsor channels and DMs winners (with delay).
func (s *Service) NotifyWinnersSelected(ctx context.Context, g *dg.Giveaway, winners []dg.Winner) {
	if s == nil || s.tg == nil || g == nil || len(winners) == 0 {
		return
	}
	// Build winners list as usernames or tg:// links
	names := make([]string, 0, len(winners))
	for _, w := range winners {
		label := ""
		if s.users != nil {
			if u, err := s.users.GetByID(ctx, w.UserID); err == nil && u != nil {
				if u.Username != "" {
					label = "@" + u.Username
				} else {
					display := u.FirstName
					if display == "" && u.LastName != "" {
						display = u.LastName
					}
					if display == "" {
						display = "User"
					}
					label = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, w.UserID, escapeHTML(display))
				}
			}
		}
		if label == "" {
			// Fallback: link with generic name
			label = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, w.UserID, "User")
		}
		names = append(names, label)
	}
	var b strings.Builder
	b.WriteString("🎉 Giveaway completed!\n\n")
	if g.Title != "" {
		b.WriteString("Title: ")
		b.WriteString(g.Title)
		b.WriteString("\n")
	}
	b.WriteString("Winners: ")
	b.WriteString(strings.Join(names, ", "))
	text := b.String()
	btnURL := s.buildWebAppURL(g.ID)

	// Post to sponsor channels
	for _, ch := range g.Sponsors {
		if ch.ID == 0 {
			continue
		}
		_ = s.tg.SendMessage(ctx, ch.ID, text, "HTML", "View Results", btnURL, true)
	}

	// DM winners with small delay between sends
	startURL := s.buildStartAppURL(g.ID)
	for i, w := range winners {
		go func(idx int, uid int64) {
			// Spread sends a bit to avoid burst
			time.Sleep(time.Duration(250+idx*150) * time.Millisecond)
			msg := fmt.Sprintf("🎉 You won in “%s”!\nOpen the app to view details.", g.Title)
			_ = s.tg.SendMessage(context.Background(), uid, msg, "HTML", "Open Giveaway", startURL, true)
		}(i, w.UserID)
	}
}

// NotifyWinnersDM sends DM notifications to winners only (no channel posts).
func (s *Service) NotifyWinnersDM(ctx context.Context, g *dg.Giveaway, winners []dg.Winner) {
	if s == nil || s.tg == nil || g == nil || len(winners) == 0 {
		return
	}
	// DM winners with small delay between sends
	startURL := s.buildStartAppURL(g.ID)
	for i, w := range winners {
		go func(idx int, uid int64) {
			// Spread sends a bit to avoid burst
			time.Sleep(time.Duration(250+idx*150) * time.Millisecond)
			msg := fmt.Sprintf("🎉 You won in “%s”!\nOpen the app to view details.", g.Title)
			_ = s.tg.SendMessage(context.Background(), uid, msg, "HTML", "Open Giveaway", startURL, true)
		}(i, w.UserID)
	}
}

// NotifyCreatorCompleted sends a DM to the giveaway creator when the giveaway is completed.
func (s *Service) NotifyCreatorCompleted(ctx context.Context, g *dg.Giveaway) {
	if s == nil || s.tg == nil || g == nil || g.CreatorID == 0 {
		return
	}
	msg := fmt.Sprintf("✅ Your giveaway \"%s\" has been completed.\n\nWinners have been selected and notified.", g.Title)
	btnURL := s.buildWebAppURL(g.ID)
	
	_ = s.tg.SendMessage(ctx, g.CreatorID, msg, "HTML", "View Giveaway", btnURL, true)
}

// NotifyCreatorPending sends a DM to the giveaway creator when the giveaway is pending and requires action.
func (s *Service) NotifyCreatorPending(ctx context.Context, g *dg.Giveaway) {
	if s == nil || s.tg == nil || g == nil || g.CreatorID == 0 {
		return
	}
	msg := fmt.Sprintf("⏳ Your giveaway \"%s\" has ended and is now pending.\n\nAction required: Please review participants, verify custom requirements, and finalize the giveaway to distribute prizes.", g.Title)
	btnURL := s.buildWebAppURL(g.ID)
	_ = s.tg.SendMessage(ctx, g.CreatorID, msg, "HTML", "Open Giveaway", btnURL, true)
}

func buildStartMessage(g *dg.Giveaway) string {
	var b strings.Builder
	b.WriteString("🎁 Giveaway is live!\n\n")
	b.WriteString("Details:\n")
	// Subscribe line: from sponsors list usernames if present
	subs := collectSponsorsUsernames(g)
	if subs != "" {
		b.WriteString("Subscribe: ")
		b.WriteString(subs)
		b.WriteString("\n")
	}
	// Deadline in UTC
	b.WriteString("Deadline: ")
	b.WriteString(g.EndsAt.UTC().Format("02 Jan 2006 15:04 UTC"))
	b.WriteString("\n")
	// Prizes
	prizes := collectPrizeTitles(g)
	if prizes != "" {
		b.WriteString("Prizes: ")
		b.WriteString(prizes)
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}
	// Requirements block
	req := buildRequirementsBlock(g)
	if req != "" {
		b.WriteString("Requirements:\n")
		b.WriteString(req)
		b.WriteString("\n")
	}
	b.WriteString("Participants can now join this giveaway. Good luck!")
	return b.String()
}

func buildCompletedMessage(g *dg.Giveaway, winnersSelected int) string {
	var b strings.Builder
	b.WriteString("🎉 Giveaway completed!\n\n")
	prizes := collectPrizeTitles(g)
	if prizes != "" {
		b.WriteString("🎁 Prizes awarded: ")
		b.WriteString(prizes)
		b.WriteString("\n\n")
	}
	b.WriteString("📊 Results:\n")
	b.WriteString(fmt.Sprintf("👥 Total participants: %d\n", g.ParticipantsCount))
	if winnersSelected > 0 {
		b.WriteString(fmt.Sprintf("🏆 Winners selected: %d\n\n", winnersSelected))
	} else {
		b.WriteString("\n")
	}
	b.WriteString("🎊 Congratulations to all the winners!")
	return b.String()
}

func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

func collectSponsorsUsernames(g *dg.Giveaway) string {
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

func collectPrizeTitles(g *dg.Giveaway) string {
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

func buildRequirementsBlock(g *dg.Giveaway) string {
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
				// Convert nano to TON with 9 decimals
				tons := float64(r.TonMinBalanceNano) / 1_000_000_000
				b.WriteString(fmt.Sprintf("• Minimum TON balance: %.4f TON\n", tons))
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
		}
	}
	return b.String()
}
