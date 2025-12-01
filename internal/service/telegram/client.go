package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	Media      map[string]string
}

func NewClientFromEnv() *Client {
	cdnURL := os.Getenv("CDN_URL")
	if cdnURL == "" {
		cdnURL = "https://tg-tools.fra1.cdn.digitaloceanspaces.com"
	}
	cdnURL = strings.TrimRight(cdnURL, "/")

	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		token:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		logger:     log.New(os.Stdout, "[TelegramClient] ", log.LstdFlags),
		Media: map[string]string{
			"giveaway_started":  fmt.Sprintf("%s/Giveaway.mp4", cdnURL),
			"giveaway_finished": fmt.Sprintf("%s/Giveaway.mp4", cdnURL),
		},
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

// SendAnimation sends an animation (GIF) to a chat/channel with optional caption and inline button.
// animation can be a file_id or an HTTP URL. parseMode can be "HTML" or "MarkdownV2".
func (c *Client) SendAnimation(ctx context.Context, chatID int64, animation string, caption string, parseMode string, buttonText string, buttonURL string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendAnimation", c.token)
	data := url.Values{
		"chat_id":   {fmt.Sprintf("%d", chatID)},
		"animation": {animation},
	}
	if caption != "" {
		data.Set("caption", caption)
	}
	if parseMode != "" {
		data.Set("parse_mode", parseMode)
	}
	if buttonText != "" && buttonURL != "" {
		markup := fmt.Sprintf(`{"inline_keyboard":[[{"text":"%s","url":"%s"}]]}`,
			escapeJSON(buttonText), escapeJSON(buttonURL))
		data.Set("reply_markup", markup)
	}
	var resp tgResponse[map[string]any]
	if err := c.makeRequest(ctx, http.MethodPost, endpoint, data, &resp); err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("telegram sendAnimation error: %s", resp.Description)
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

// SavePreparedInlineMessageArticle calls savePreparedInlineMessage with an InlineQueryResultArticle payload
// and returns the prepared inline message ID. See Telegram docs:
// https://core.telegram.org/bots/api#savepreparedinlinemessage
func (c *Client) SavePreparedInlineMessageArticle(ctx context.Context, userID int64, title string, messageHTML string, buttonText string, buttonURL string) (string, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/savePreparedInlineMessage", c.token)
	// Build InlineKeyboard if provided
	var replyMarkup any
	if buttonText != "" && buttonURL != "" {
		replyMarkup = map[string]any{
			"inline_keyboard": [][]map[string]string{
				{
					{"text": buttonText, "url": buttonURL},
				},
			},
		}
	}
	// InlineQueryResultArticle with minimal required fields
	result := map[string]any{
		"type":  "article",
		"id":    fmt.Sprintf("g-%d-%d", userID, time.Now().UnixNano()),
		"title": title,
		"input_message_content": map[string]any{
			"message_text": messageHTML,
			"parse_mode":   "HTML",
		},
	}
	if replyMarkup != nil {
		result["reply_markup"] = replyMarkup
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	data := url.Values{
		"user_id": {fmt.Sprintf("%d", userID)},
		"result":  {string(resultJSON)},
	}
	// Telegram requires at least one allowed chat type
	data.Set("allow_user_chats", "true")
	data.Set("allow_group_chats", "true")
	data.Set("allow_channel_chats", "true")
	// Log request (without token)
	if c.logger != nil {
		c.logger.Printf("Telegram: savePreparedInlineMessage request user_id=%d title=%q button_text=%q button_url=%q", userID, title, buttonText, buttonURL)
	}
	// Response may return a struct with id or inline_message_id; be tolerant
	var resp struct {
		Ok          bool           `json:"ok"`
		Description string         `json:"description"`
		Result      map[string]any `json:"result"`
	}
	if err := c.makeRequest(ctx, http.MethodPost, endpoint, data, &resp); err != nil {
		if c.logger != nil {
			c.logger.Printf("Telegram: savePreparedInlineMessage error: %v", err)
		}
		return "", err
	}
	if !resp.Ok {
		if resp.Description == "" {
			resp.Description = "telegram API error"
		}
		if c.logger != nil {
			raw, _ := json.Marshal(resp.Result)
			c.logger.Printf("Telegram: savePreparedInlineMessage failed: %s; result=%s", resp.Description, string(raw))
		}
		return "", fmt.Errorf(resp.Description)
	}
	// Extract id field
	if resp.Result == nil {
		if c.logger != nil {
			c.logger.Printf("Telegram: savePreparedInlineMessage empty result")
		}
		return "", fmt.Errorf("empty result")
	}
	// Common possible keys
	var extracted string
	for _, key := range []string{"id", "inline_message_id", "prepared_inline_message_id"} {
		if v, ok := resp.Result[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				extracted = s
				break
			}
		}
	}
	// Fallback: try stringify first non-empty string value
	if extracted == "" {
		for _, v := range resp.Result {
			if s, ok := v.(string); ok && s != "" {
				extracted = s
				break
			}
		}
	}
	if extracted == "" {
		if c.logger != nil {
			raw, _ := json.Marshal(resp.Result)
			c.logger.Printf("Telegram: savePreparedInlineMessage id not found in result=%s", string(raw))
		}
		return "", fmt.Errorf("prepared inline message id not found")
	}
	if c.logger != nil {
		raw, _ := json.Marshal(resp.Result)
		c.logger.Printf("Telegram: savePreparedInlineMessage success id=%s result=%s", extracted, string(raw))
	}
	return extracted, nil
}

// SavePreparedInlineMessageGif creates a prepared inline message using an animated GIF with a caption.
// This mimics SendAnimation used elsewhere, but via savePreparedInlineMessage with InlineQueryResultGif.
func (c *Client) SavePreparedInlineMessageGif(ctx context.Context, userID int64, gifURL string, thumbnailURL string, captionHTML string, buttonText string, buttonURL string) (string, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/savePreparedInlineMessage", c.token)
	// Inline keyboard
	var replyMarkup any
	if buttonText != "" && buttonURL != "" {
		replyMarkup = map[string]any{
			"inline_keyboard": [][]map[string]string{
				{
					{"text": buttonText, "url": buttonURL},
				},
			},
		}
	}
	// InlineQueryResultGif payload
	result := map[string]any{
		"type": "mpeg4_gif",
		"id":   fmt.Sprintf("g-%d-%d", userID, time.Now().UnixNano()),
	}

	// If gifURL looks like a file_id, use mpeg4_file_id, otherwise use mpeg4_url
	if strings.HasPrefix(gifURL, "http") {
		result["mpeg4_url"] = gifURL
		// For MPEG4_GIF via URL, thumb_url/thumbnail_url is technically optional if not required by clients,
		// but sometimes required if Telegram can't generate it.
		// We can reuse the same URL as thumb for MP4 if it's small enough, or omit.
		// For reliability, if we have a separate thumb, use it.
		if thumbnailURL != "" {
			result["thumbnail_url"] = thumbnailURL
		} else {
			// Use the same URL as thumbnail if it's a URL, hoping it works or is not strictly required for mpeg4_gif
			result["thumbnail_url"] = gifURL
		}
	} else {
		// It's a file_id
		result["mpeg4_file_id"] = gifURL
	}

	if captionHTML != "" {
		result["caption"] = captionHTML
		result["parse_mode"] = "HTML"
	}
	if replyMarkup != nil {
		result["reply_markup"] = replyMarkup
	}
	body, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	data := url.Values{
		"user_id": {fmt.Sprintf("%d", userID)},
		"result":  {string(body)},
	}
	// Allow in main chat types
	data.Set("allow_user_chats", "true")
	data.Set("allow_group_chats", "true")
	data.Set("allow_channel_chats", "true")
	if c.logger != nil {
		c.logger.Printf("Telegram: savePreparedInlineMessage (gif) request user_id=%d gif_url=%q", userID, gifURL)
	}
	var resp struct {
		Ok          bool           `json:"ok"`
		Description string         `json:"description"`
		Result      map[string]any `json:"result"`
	}
	if err := c.makeRequest(ctx, http.MethodPost, endpoint, data, &resp); err != nil {
		if c.logger != nil {
			c.logger.Printf("Telegram: savePreparedInlineMessage (gif) error: %v", err)
		}
		return "", err
	}
	if !resp.Ok {
		if resp.Description == "" {
			resp.Description = "telegram API error"
		}
		if c.logger != nil {
			raw, _ := json.Marshal(resp.Result)
			c.logger.Printf("Telegram: savePreparedInlineMessage (gif) failed: %s; result=%s", resp.Description, string(raw))
		}
		return "", fmt.Errorf(resp.Description)
	}
	if resp.Result == nil {
		return "", fmt.Errorf("empty result")
	}
	// Extract ID
	for _, key := range []string{"id", "inline_message_id", "prepared_inline_message_id"} {
		if v, ok := resp.Result[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				if c.logger != nil {
					c.logger.Printf("Telegram: savePreparedInlineMessage (gif) success, %s=%s", key, s)
				}
				return s, nil
			}
		}
	}
	// Fallback
	for _, v := range resp.Result {
		if s, ok := v.(string); ok && s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("prepared inline message id not found")
}

// UploadAnimation uploads a local animation file to Telegram via multipart/form-data and returns the file_id.
func (c *Client) UploadAnimation(ctx context.Context, chatID int64, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("animation", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(part, file); err != nil {
		return "", err
	}
	if err = writer.WriteField("chat_id", fmt.Sprintf("%d", chatID)); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendAnimation", c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			Animation struct {
				FileID string `json:"file_id"`
			} `json:"animation"`
			Document struct {
				FileID string `json:"file_id"`
			} `json:"document"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if !result.Ok {
		return "", fmt.Errorf("telegram upload error: %s", result.Description)
	}

	if result.Result.Animation.FileID != "" {
		return result.Result.Animation.FileID, nil
	}
	if result.Result.Document.FileID != "" {
		return result.Result.Document.FileID, nil
	}

	return "", fmt.Errorf("no file_id found in response")
}
