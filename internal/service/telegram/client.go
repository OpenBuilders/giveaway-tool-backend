package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
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

// GetPublicChannelInfoByID fetches public info by numeric channel id using Telegram API.
func (c *Client) GetPublicChannelInfoByID(ctx context.Context, id int64) (*PublicChannelInfo, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChat", c.token)
	params := url.Values{"chat_id": {fmt.Sprintf("%d", id)}}

	var result tgResponse[chat]
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, params, &result); err != nil {
		return nil, fmt.Errorf("getChat: %w", err)
	}
	if !result.Ok {
		return nil, fmt.Errorf("telegram API error: %s", result.Description)
	}

	username := result.Result.Username
	var avatarURL string
	if username != "" {
		avatarURL = fmt.Sprintf("https://t.me/i/userpic/160/%s.jpg", username)
		req, _ := http.NewRequestWithContext(ctx, http.MethodHead, avatarURL, nil)
		resp, err := c.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				avatarURL = ""
			}
		}
	}

	return &PublicChannelInfo{
		ID:       result.Result.ID,
		Username: username,
		ChannelURL: func() string {
			if username == "" {
				return ""
			}
			return fmt.Sprintf("https://t.me/%s", username)
		}(),
		AvatarURL: avatarURL,
		Title:     result.Result.Title,
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
