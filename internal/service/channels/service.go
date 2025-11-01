package channels

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	rplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
	"github.com/redis/go-redis/v9"
)

// Channel holds minimal channel info via Redis storage
type Channel struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Username      string `json:"username"`
	URL           string `json:"url,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	PhotoSmallURL string `json:"photo_small_url,omitempty"`
}

// Service provides access to Telegram channel data stored in Redis.
type Service struct {
	rdb *rplatform.Client
}

func NewService(rdb *rplatform.Client) *Service { return &Service{rdb: rdb} }

// GetByID returns channel info by numeric id from Redis keys
// channel:{id}:title, channel:{id}:username, channel:{id}:url. Missing keys yield empty fields.
func (s *Service) GetByID(ctx context.Context, id int64) (*Channel, error) {
	title, _ := s.rdb.Get(ctx, fmt.Sprintf("channel:%d:title", id)).Result()
	username, _ := s.rdb.Get(ctx, fmt.Sprintf("channel:%d:username", id)).Result()
	urlVal, _ := s.rdb.Get(ctx, fmt.Sprintf("channel:%d:url", id)).Result()
	photoSmall, _ := s.rdb.Get(ctx, fmt.Sprintf("channel:%d:photo_small_url", id)).Result()
	avatar := buildAvatarURL(username, title)
	return &Channel{ID: id, Title: title, Username: username, URL: urlVal, AvatarURL: avatar, PhotoSmallURL: photoSmall}, nil
}

// ListUserChannels returns all channels for a user by reading set user:{id}:channels
// and resolving title/username/url for each channel id.
func (s *Service) ListUserChannels(ctx context.Context, userID int64) ([]Channel, error) {
	key := fmt.Sprintf("user:%d:channels", userID)
	members, err := s.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []Channel{}, nil
	}

	// Pipeline title, username and url lookups
	pipe := s.rdb.Pipeline()
	titleCmds := make([]*redis.StringCmd, len(members))
	usernameCmds := make([]*redis.StringCmd, len(members))
	urlCmds := make([]*redis.StringCmd, len(members))
	photoSmallCmds := make([]*redis.StringCmd, len(members))
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
		urlCmds[i] = pipe.Get(ctx, fmt.Sprintf("channel:%d:url", id))
		photoSmallCmds[i] = pipe.Get(ctx, fmt.Sprintf("channel:%d:photo_small_url", id))
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
		urlVal, _ := urlCmds[i].Result()
		photoSmall, _ := photoSmallCmds[i].Result()
		avatar := buildAvatarURL(username, title)
		out = append(out, Channel{ID: id, Title: title, Username: username, URL: urlVal, AvatarURL: avatar, PhotoSmallURL: photoSmall})
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
