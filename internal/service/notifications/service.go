package notifications

import (
	"context"
	"fmt"
	"strings"

	dg "github.com/open-builders/giveaway-backend/internal/domain/giveaway"
	"github.com/open-builders/giveaway-backend/internal/service/channels"
	tg "github.com/open-builders/giveaway-backend/internal/service/telegram"
)

// Service formats and sends giveaway notifications to creator channels.
type Service struct {
	tg         *tg.Client
	channels   *channels.Service
	webAppBase string
}

func NewService(tgc *tg.Client, chs *channels.Service, webAppBaseURL string) *Service {
	return &Service{tg: tgc, channels: chs, webAppBase: strings.TrimRight(webAppBaseURL, "/")}
}

// NotifyStarted posts an announcement to all creator channels when a giveaway starts.
func (s *Service) NotifyStarted(ctx context.Context, g *dg.Giveaway) {
	if s == nil || s.tg == nil || s.channels == nil || g == nil || g.CreatorID == 0 {
		return
	}
	// Build message
	text := buildStartMessage(g)
	btnURL := s.buildWebAppURL(g.ID)
	// Deliver to each creator channel
	chs, err := s.channels.ListUserChannels(ctx, g.CreatorID)
	if err != nil {
		return
	}
	for _, ch := range chs {
		if ch.ID == 0 {
			continue
		}
		_ = s.tg.SendMessage(ctx, ch.ID, text, "HTML", "Open Giveaway", btnURL, true)
	}
}

// NotifyCompleted posts results to all creator channels when a giveaway completes.
func (s *Service) NotifyCompleted(ctx context.Context, g *dg.Giveaway, winnersSelected int) {
	if s == nil || s.tg == nil || s.channels == nil || g == nil || g.CreatorID == 0 {
		return
	}
	text := buildCompletedMessage(g, winnersSelected)
	btnURL := s.buildWebAppURL(g.ID)
	chs, err := s.channels.ListUserChannels(ctx, g.CreatorID)
	if err != nil {
		return
	}
	for _, ch := range chs {
		if ch.ID == 0 {
			continue
		}
		_ = s.tg.SendMessage(ctx, ch.ID, text, "HTML", "View Results", btnURL, true)
	}
}

func (s *Service) buildWebAppURL(id string) string {
	if s.webAppBase == "" {
		return ""
	}
	return fmt.Sprintf("%s/g/%s", s.webAppBase, id)
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
