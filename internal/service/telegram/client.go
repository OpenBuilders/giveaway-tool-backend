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

	rplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
	tgutils "github.com/open-builders/giveaway-backend/internal/utils/telegram"
)

// Client provides minimal Telegram API utilities used by the backend.
type Client struct {
	httpClient *http.Client
	token      string
	logger     *log.Logger
	botID      int64
}

func NewClientFromEnv() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		token:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		logger:     log.New(os.Stdout, "[TelegramClient] ", log.LstdFlags),
	}
}

type chat struct {
	ID       int64      `json:"id"`
	Type     string     `json:"type"`
	Title    string     `json:"title"`
	Username string     `json:"username"`
	Photo    *chatPhoto `json:"photo,omitempty"`
}

type chatPhoto struct {
	SmallFileID       string `json:"small_file_id"`
	SmallFileUniqueID string `json:"small_file_unique_id"`
	BigFileID         string `json:"big_file_id"`
	BigFileUniqueID   string `json:"big_file_unique_id"`
}

type tgResponse[T any] struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      T      `json:"result"`
}

type user struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

// BotMe stores minimal bot info cached in Redis.
type BotMe struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	IsBot    bool   `json:"is_bot"`
}

// SetBotMe fetches current bot info via getMe and stores it in Redis.
// Also sets a convenience key for username.
func (c *Client) SetBotMe(ctx context.Context, rdb *rplatform.Client) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", c.token)
	var resp tgResponse[user]
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, nil, &resp); err != nil {
		return err
	}
	if !resp.Ok || resp.Result.ID == 0 {
		if resp.Description != "" {
			return fmt.Errorf("getMe failed: %s", resp.Description)
		}
		return fmt.Errorf("getMe failed")
	}
	me := BotMe{ID: resp.Result.ID, Username: resp.Result.Username, IsBot: resp.Result.IsBot}
	payload, err := json.Marshal(me)
	if err != nil {
		return err
	}
	if err := rdb.Set(ctx, "bot:me", payload, 0).Err(); err != nil {
		return err
	}
	if me.Username != "" {
		_ = rdb.Set(ctx, "bot:username", me.Username, 0).Err()
	}
	// cache locally as well
	c.botID = me.ID
	return nil
}

// GetBotMe returns cached bot info from Redis; if missing, it calls SetBotMe first.
func (c *Client) GetBotMe(ctx context.Context, rdb *rplatform.Client) (*BotMe, error) {
	v, err := rdb.Get(ctx, "bot:me").Bytes()
	if err == nil && len(v) > 0 {
		var me BotMe
		if jerr := json.Unmarshal(v, &me); jerr == nil && me.ID != 0 {
			return &me, nil
		}
	}
	if err := c.SetBotMe(ctx, rdb); err != nil {
		return nil, err
	}
	v, err = rdb.Get(ctx, "bot:me").Bytes()
	if err != nil {
		return nil, err
	}
	var me BotMe
	if jerr := json.Unmarshal(v, &me); jerr != nil {
		return nil, jerr
	}
	return &me, nil
}

// PublicChannelInfo is the response DTO for GET channel info.
type PublicChannelInfo struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"`
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

	avatarURL := tgutils.BuildAvatarURL(strconv.FormatInt(result.Result.ID, 10))
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
		Type:       result.Result.Type,
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
		avatarURL = tgutils.BuildAvatarURL(strconv.FormatInt(result.Result.ID, 10))
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
		Type:     result.Result.Type,
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

// BuildFileURL returns the absolute Bot API file URL for a given file_path.
func (c *Client) BuildFileURL(filePath string) string {
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.token, filePath)
}

// GetChatRaw wraps getChat and returns raw chat payload for either @username or numeric id.
func (c *Client) GetChatRaw(ctx context.Context, chatRef string) (*chat, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChat", c.token)
	params := url.Values{"chat_id": {chatRef}}
	var result tgResponse[chat]
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, params, &result); err != nil {
		return nil, fmt.Errorf("getChat: %w", err)
	}
	if !result.Ok {
		return nil, fmt.Errorf("telegram API error: %s", result.Description)
	}
	return &result.Result, nil
}

// GetFilePath wraps getFile and returns the file_path for a given file_id.
func (c *Client) GetFilePath(ctx context.Context, fileID string) (string, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getFile", c.token)
	params := url.Values{"file_id": {fileID}}
	var result struct {
		Ok     bool   `json:"ok"`
		Error  string `json:"description"`
		Result struct {
			FileID   string `json:"file_id"`
			FileSize int64  `json:"file_size"`
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, params, &result); err != nil {
		return "", fmt.Errorf("getFile: %w", err)
	}
	if !result.Ok {
		if result.Error == "" {
			result.Error = "telegram API error"
		}
		return "", fmt.Errorf(result.Error)
	}
	return result.Result.FilePath, nil
}

// SendMessage sends a message to a chat/channel with optional inline button using Telegram Bot API.
// If buttonText and buttonURL are non-empty, an inline keyboard with a single button is attached.
// parseMode can be "HTML" or "MarkdownV2"; empty means no parse mode.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, parseMode string, buttonText string, buttonURL string, disablePreview bool) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.token)
	data := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"text":    {text},
	}
	if parseMode != "" {
		data.Set("parse_mode", parseMode)
	}
	if disablePreview {
		data.Set("disable_web_page_preview", "true")
	}
	if buttonText != "" && buttonURL != "" {
		// Minimal inline keyboard with one button
		markup := fmt.Sprintf(`{"inline_keyboard":[[{"text":"%s","url":"%s"}]]}`,
			escapeJSON(buttonText), escapeJSON(buttonURL))
		data.Set("reply_markup", markup)
	}
	var resp tgResponse[map[string]any]
	if err := c.makeRequest(ctx, http.MethodPost, endpoint, data, &resp); err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("telegram sendMessage error: %s", resp.Description)
	}
	return nil
}

// escapeJSON performs a minimal escape for quotes and backslashes used in inline JSON strings.
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\\\\`)
	s = strings.ReplaceAll(s, `"`, `\\\"`)
	return s
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

// CheckBoost checks whether the user has any active boosts in the chat.
// chatID may be @username or numeric id as string.
func (c *Client) CheckBoost(ctx context.Context, userID int64, chatID string) (bool, error) {
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

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUserChatBoosts", c.token)
	data := url.Values{
		"chat_id": {fmt.Sprintf("%d", numericChatID)},
		"user_id": {fmt.Sprintf("%d", userID)},
	}

	var response struct {
		Ok     bool   `json:"ok"`
		Error  string `json:"error"`
		Result struct {
			Boosts []any `json:"boosts"`
		} `json:"result"`
	}
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, data, &response); err != nil {
		return false, fmt.Errorf("failed to check boost status: %w", err)
	}
	if !response.Ok {
		if strings.Contains(response.Error, "Too Many Requests") {
			return false, fmt.Errorf("rate limit exceeded")
		}
		return false, fmt.Errorf("telegram API error: %s", response.Error)
	}
	return len(response.Result.Boosts) > 0, nil
}

// ensureBotID retrieves and caches the bot's own user ID via getMe.
func (c *Client) ensureBotID(ctx context.Context) (int64, error) {
	if c.botID != 0 {
		return c.botID, nil
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", c.token)
	var resp tgResponse[user]
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, nil, &resp); err != nil {
		return 0, err
	}
	if !resp.Ok || resp.Result.ID == 0 {
		if resp.Description != "" {
			return 0, fmt.Errorf("getMe failed: %s", resp.Description)
		}
		return 0, fmt.Errorf("getMe failed")
	}
	c.botID = resp.Result.ID
	return c.botID, nil
}

// IsBotMember checks whether the bot is a member/admin/creator of the chat.
// chat can be @username or numeric id as string.
func (c *Client) IsBotMember(ctx context.Context, chat string) (bool, error) {
	botID, err := c.ensureBotID(ctx)

	if err != nil {
		return false, err
	}

	var numericChatID int64
	if len(chat) > 0 && chat[0] == '@' {
		ch, err := c.GetPublicChannelInfo(ctx, chat)
		if err != nil {
			return false, fmt.Errorf("failed to get chat info: %w", err)
		}
		numericChatID = ch.ID
	} else {
		id, err := strconv.ParseInt(chat, 10, 64)
		if err != nil {
			// treat as username without @
			ch, err := c.GetPublicChannelInfo(ctx, chat)
			if err != nil {
				return false, fmt.Errorf("failed to get chat info: %w", err)
			}
			numericChatID = ch.ID
		} else {
			numericChatID = id
		}
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", c.token)
	data := url.Values{
		"chat_id": {fmt.Sprintf("%d", numericChatID)},
		"user_id": {fmt.Sprintf("%d", botID)},
	}
	var response struct {
		Ok     bool       `json:"ok"`
		Error  string     `json:"error"`
		Result ChatMember `json:"result"`
	}
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, data, &response); err != nil {
		return false, fmt.Errorf("failed to check bot membership: %w", err)
	}
	if !response.Ok {
		if strings.Contains(response.Error, "Too Many Requests") {
			return false, fmt.Errorf("rate limit exceeded")
		}

		return false, fmt.Errorf("Bot is not a member of the chat")
	}
	switch response.Result.Status {
	case "creator", "administrator", "member", "restricted":
		return true, nil
	default:
		return false, nil
	}
}

// GetBotMemberStatus returns raw membership status for the bot and whether it can check members
// (true when bot is administrator or has permissions to manage chat).
func (c *Client) GetBotMemberStatus(ctx context.Context, chat string) (string, bool, error) {
	botID, err := c.ensureBotID(ctx)
	if err != nil {
		return "", false, err
	}
	var numericChatID int64
	if len(chat) > 0 && chat[0] == '@' {
		ch, err := c.GetPublicChannelInfo(ctx, chat)
		if err != nil {
			return "", false, fmt.Errorf("failed to get chat info: %w", err)
		}
		numericChatID = ch.ID
	} else {
		id, err := strconv.ParseInt(chat, 10, 64)
		if err != nil {
			ch, err := c.GetPublicChannelInfo(ctx, chat)
			if err != nil {
				return "", false, fmt.Errorf("failed to get chat info: %w", err)
			}
			numericChatID = ch.ID
		} else {
			numericChatID = id
		}
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", c.token)
	data := url.Values{
		"chat_id": {fmt.Sprintf("%d", numericChatID)},
		"user_id": {fmt.Sprintf("%d", botID)},
	}
	var response struct {
		Ok     bool   `json:"ok"`
		Error  string `json:"error"`
		Result struct {
			Status string `json:"status"`
			// admin permissions (subset); presence implies can_check_members true
			CanInviteUsers     bool `json:"can_invite_users"`
			CanDeleteMessages  bool `json:"can_delete_messages"`
			CanRestrictMembers bool `json:"can_restrict_members"`
			CanManageChat      bool `json:"can_manage_chat"`
		} `json:"result"`
	}
	if err := c.makeRequest(ctx, http.MethodGet, endpoint, data, &response); err != nil {
		return "", false, fmt.Errorf("failed to check bot membership: %w", err)
	}
	if !response.Ok {
		if strings.Contains(response.Error, "Too Many Requests") {
			return "", false, fmt.Errorf("rate limit exceeded")
		}
		return "", false, fmt.Errorf("telegram API error: %s", response.Error)
	}
	status := response.Result.Status
	can := status == "administrator" || status == "creator" || response.Result.CanManageChat || response.Result.CanRestrictMembers || response.Result.CanDeleteMessages
	return status, can, nil
}
