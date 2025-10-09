package channels

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/redis/go-redis/v9"
	rplatform "github.com/your-org/giveaway-backend/internal/platform/redis"
)

// Channel holds minimal channel info via Redis storage
type Channel struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Service provides access to Telegram channel data stored in Redis.
type Service struct {
	rdb *rplatform.Client
}

func NewService(rdb *rplatform.Client) *Service { return &Service{rdb: rdb} }

// GetByID returns channel info by numeric id from Redis keys
// channel:{id}:title and channel:{id}:username. Missing keys yield empty fields.
func (s *Service) GetByID(ctx context.Context, id int64) (*Channel, error) {
	title, _ := s.rdb.Get(ctx, fmt.Sprintf("channel:%d:title", id)).Result()
	username, _ := s.rdb.Get(ctx, fmt.Sprintf("channel:%d:username", id)).Result()
	avatar := buildAvatarURL(username, title)
	return &Channel{ID: id, Title: title, Username: username, AvatarURL: avatar}, nil
}

// ListUserChannels returns all channels for a user by reading set user:{id}:channels
// and resolving title/username for each channel id.
func (s *Service) ListUserChannels(ctx context.Context, userID int64) ([]Channel, error) {
	key := fmt.Sprintf("user:%d:channels", userID)
	members, err := s.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []Channel{}, nil
	}

	// Pipeline title and username lookups
	pipe := s.rdb.Pipeline()
	titleCmds := make([]*redis.StringCmd, len(members))
	usernameCmds := make([]*redis.StringCmd, len(members))
	chanIDs := make([]int64, len(members))
	for i, m := range members {
		id, convErr := strconv.ParseInt(m, 10, 64)
		if convErr != nil {
			// Skip invalid entries
			continue
		}
		chanIDs[i] = id
		titleCmds[i] = pipe.Get(ctx, fmt.Sprintf("channel:%d:title", id))
		usernameCmds[i] = pipe.Get(ctx, fmt.Sprintf("channel:%d:username", id))
	}
	if _, err := pipe.Exec(ctx); err != nil && err.Error() != "redis: nil" {
		// Ignore missing keys by not treating redis.Nil as fatal
		// Note: go-redis aggregates; we keep best-effort resolution
	}

	out := make([]Channel, 0, len(members))
	for i, id := range chanIDs {
		if id == 0 {
			continue
		}
		title, _ := titleCmds[i].Result()
		username, _ := usernameCmds[i].Result()
		avatar := buildAvatarURL(username, title)
		out = append(out, Channel{ID: id, Title: title, Username: username, AvatarURL: avatar})
	}
	return out, nil
}

// buildAvatarURL prefers Telegram's public avatar URL by username; falls back to placeholder by title.
func buildAvatarURL(username, title string) string {
	if username != "" {
		return fmt.Sprintf("https://t.me/i/userpic/160/%s.jpg", username)
	}
	if title == "" {
		return ""
	}
	return "https://ui-avatars.com/api/?background=random&size=128&name=" + url.QueryEscape(title)
}
