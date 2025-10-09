package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client provides minimal Telegram API utilities used by the backend.
type Client struct {
	httpClient *http.Client
	token      string
	logger     *log.Logger
}

func NewClientFromEnv() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		token:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		logger:     log.New(os.Stdout, "[TelegramClient] ", log.LstdFlags),
	}
}

type chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type tgResponse[T any] struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      T      `json:"result"`
}

// PublicChannelInfo is the response DTO for GET channel info.
type PublicChannelInfo struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	ChannelURL string `json:"channel_url"`
	AvatarURL  string `json:"avatar_url"`
	Title      string `json:"title"`
}

// GetPublicChannelInfo fetches public info by @username using Telegram API.
func (c *Client) GetPublicChannelInfo(ctx context.Context, username string) (*PublicChannelInfo, error) {
	username = strings.TrimPrefix(username, "@")
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChat", c.token)
	params := url.Values{"chat_id": {"@" + username}}

	var result tgResponse[chat]
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, params, &result); err != nil {
		return nil, fmt.Errorf("getChat: %w", err)
	}
	if !result.Ok {
		return nil, fmt.Errorf("telegram API error: %s", result.Description)
	}

	avatarURL := fmt.Sprintf("https://t.me/i/userpic/160/%s.jpg", username)
	// best-effort HEAD to check existence
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, avatarURL, nil)
	resp, err := c.httpClient.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			avatarURL = ""
		} else if resp.StatusCode != http.StatusOK {
			avatarURL = ""
		}
	}

	return &PublicChannelInfo{
		ID:         result.Result.ID,
		Username:   username,
		ChannelURL: fmt.Sprintf("https://t.me/%s", username),
		AvatarURL:  avatarURL,
		Title:      result.Result.Title,
	}, nil
}

func (c *Client) makeRequest(ctx context.Context, method, endpoint string, data url.Values, out any) error {
	var req *http.Request
	var err error
	if method == http.MethodPost {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(data.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		if len(data) > 0 {
			endpoint = endpoint + "?" + data.Encode()
		}
		req, err = http.NewRequestWithContext(ctx, method, endpoint, nil)
		if err != nil {
			return err
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}

// ChatMember minimal subset for membership checks
type ChatMember struct {
	Status string `json:"status"`
}

// CheckMembership verifies whether the user is a member/admin/creator of a chat
// chatID can be numeric id (as string) or @username
func (c *Client) CheckMembership(ctx context.Context, userID int64, chatID string) (bool, error) {
	var numericChatID int64
	if len(chatID) > 0 && chatID[0] == '@' {
		ch, err := c.GetPublicChannelInfo(ctx, chatID)
		if err != nil {
			return false, fmt.Errorf("failed to get chat info: %w", err)
		}
		numericChatID = ch.ID
	} else {
		id, err := strconv.ParseInt(chatID, 10, 64)
		if err != nil {
			return false, fmt.Errorf("invalid chat ID format: %w", err)
		}
		numericChatID = id
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", c.token)
	data := url.Values{
		"chat_id": {fmt.Sprintf("%d", numericChatID)},
		"user_id": {fmt.Sprintf("%d", userID)},
	}

	var response struct {
		Ok     bool       `json:"ok"`
		Error  string     `json:"error"`
		Result ChatMember `json:"result"`
	}

	if err := c.makeRequest(ctx, http.MethodGet, endpoint, data, &response); err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}

	if !response.Ok {
		if strings.Contains(response.Error, "Too Many Requests") {
			return false, fmt.Errorf("rate limit exceeded")
		}
		return false, fmt.Errorf("telegram API error: %s", response.Error)
	}

	switch response.Result.Status {
	case "creator", "administrator", "member", "restricted":
		return true, nil
	default:
		return false, nil
	}
}
