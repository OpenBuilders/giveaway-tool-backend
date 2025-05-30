package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
	token      string
	logger     *log.Logger
}

type RPSError struct {
	Description string
}

func (e *RPSError) Error() string {
	return fmt.Sprintf("telegram RPS error: %s", e.Description)
}

type ChatMember struct {
	Status         string `json:"status"`
	CanInviteUsers bool   `json:"can_invite_users"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type apiResponse struct {
	Ok          bool        `json:"ok"`
	Description string      `json:"description,omitempty"`
	Result      interface{} `json:"result,omitempty"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		token:  os.Getenv("BOT_TOKEN"),
		logger: log.New(os.Stdout, "[TelegramClient] ", log.LstdFlags),
	}
}

// ValidateRequirements проверяет доступность бота в указанных чатах
func (c *Client) ValidateRequirements(requirements *models.Requirements) ([]error, error) {
	var errors []error
	for _, req := range requirements.Requirements {
		// Получаем информацию о формате ID чата
		chatInfo, err := req.GetChatIDInfo()
		if err != nil {
			return nil, fmt.Errorf("invalid chat_id format: %w", err)
		}

		// Получаем числовой ID через API, если передан юзернейм
		var numericChatID int64
		if chatInfo.IsNumeric {
			numericChatID = chatInfo.NumericID
		} else {
			numericChatID, err = c.GetChatIDByUsername(chatInfo.Username)
			if err != nil {
				errors = append(errors, fmt.Errorf("chat %s is not accessible: %w", chatInfo.RawID, err))
				continue
			}
		}

		// Проверяем права бота в чате
		chatMember, err := c.GetBotChatMember(fmt.Sprintf("%d", numericChatID))
		if err != nil {
			c.logger.Printf("Failed to get bot member info: %v", err)
			errors = append(errors, fmt.Errorf("failed to check bot permissions in chat %s: %w", chatInfo.RawID, err))
			continue
		}

		if !chatMember.CanInviteUsers {
			errors = append(errors, fmt.Errorf("bot needs invite users permission in chat %s", chatInfo.RawID))
		}
	}

	return errors, nil
}

// CheckRequirements проверяет выполнение требований пользователем
func (c *Client) CheckRequirements(ctx context.Context, userID int64, requirements *models.Requirements) (bool, error) {
	for _, req := range requirements.Requirements {
		// Получаем информацию о формате ID чата
		chatInfo, err := req.GetChatIDInfo()
		if err != nil {
			return false, fmt.Errorf("invalid chat_id format: %w", err)
		}

		// Получаем числовой ID через API, если передан юзернейм
		var numericChatID int64
		if chatInfo.IsNumeric {
			numericChatID = chatInfo.NumericID
		} else {
			numericChatID, err = c.GetChatIDByUsername(chatInfo.Username)
			if err != nil {
				return false, fmt.Errorf("failed to get chat info: %w", err)
			}
		}

		switch req.Type {
		case models.RequirementTypeSubscription:
			isMember, err := c.checkChatMembership(ctx, userID, numericChatID)
			if err != nil {
				// Если получили ошибку RPS, пропускаем проверку
				var rpsErr *RPSError
				if ok := errors.As(err, &rpsErr); ok {
					return true, nil
				}
				return false, err
			}
			if !isMember {
				return false, nil
			}
		default:
			// Handle any other requirement types
		}
	}

	return true, nil
}

func (c *Client) NotifyWinner(userID int64, giveaway *models.Giveaway, place int, prize models.PrizeDetail) error {
	c.logger.Printf("Sending notification to winner %d for giveaway %s (place %d)", userID, giveaway.ID, place)

	message := fmt.Sprintf(
		"🎉 Поздравляем! Вы заняли %d место в розыгрыше \"%s\"!\n\n"+
			"🎁 Ваш приз: %s\n"+
			"📝 Описание приза: %s\n\n",
		place,
		giveaway.Title,
		prize.Name,
		prize.Description,
	)

	if prize.IsInternal {
		message += "💫 Приз будет выдан автоматически в ближайшее время."
	} else {
		message += "👥 Организатор розыгрыша свяжется с вами для передачи приза."
	}

	c.logger.Printf("Notification message: %s", message)

	_, err := c.sendMessage(userID, message)
	if err != nil {
		c.logger.Printf("Failed to send notification to winner %d: %v", userID, err)
		return err
	}

	c.logger.Printf("Successfully sent notification to winner %d", userID)
	return nil
}

func (c *Client) NotifyCreator(userID int64, giveaway *models.Giveaway) (*apiResponse, error) {
	c.logger.Printf("Sending notification to creator %d for giveaway %s", userID, giveaway.ID)

	message := fmt.Sprintf(
		"✨ Розыгрыш успешно создан!\n\n"+
			"📋 Название: %s\n"+
			"📝 Описание: %s\n"+
			"⏰ Длительность: %d секунд\n"+
			"👥 Количество победителей: %d\n\n"+
			"🎯 Статус: %s",
		giveaway.Title,
		giveaway.Description,
		giveaway.Duration,
		giveaway.WinnersCount,
		giveaway.Status,
	)

	if giveaway.MaxParticipants > 0 {
		message += fmt.Sprintf("\n👥 Максимум участников: %d", giveaway.MaxParticipants)
	}

	response, err := c.sendMessage(userID, message)
	if err != nil {
		c.logger.Printf("Failed to send notification to creator %d: %v", userID, err)
		return nil, err
	}

	c.logger.Printf("Successfully sent notification to creator %d", userID)
	return response, nil
}

// Вспомогательные методы

// GetChat retrieves information about a chat by username or ID
func (c *Client) GetChat(chatID string) (*Chat, error) {
	c.logger.Printf("Getting chat info for %s", chatID)

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChat", c.token)
	params := url.Values{
		"chat_id": {chatID},
	}

	var result struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description,omitempty"`
		Result      Chat   `json:"result"`
	}

	if err := c.makeRequest("GET", endpoint, params, &result); err != nil {
		c.logger.Printf("Failed to get chat info for %s: %v", chatID, err)
		return nil, fmt.Errorf("failed to get chat info: %w", err)
	}

	if !result.Ok {
		c.logger.Printf("Telegram API error for chat %s: %s", chatID, result.Description)
		return nil, fmt.Errorf("telegram API error: %s", result.Description)
	}

	c.logger.Printf("Successfully got chat info for %s: ID=%d, Type=%s, Username=%s",
		chatID, result.Result.ID, result.Result.Type, result.Result.Username)
	return &result.Result, nil
}

// GetBotChatMember checks if the bot is a member of the specified chat and returns its status
func (c *Client) GetBotChatMember(chatID string) (*ChatMember, error) {
	c.logger.Printf("Getting bot membership info for chat %s", chatID)

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", c.token)
	params := url.Values{
		"chat_id": {chatID},
		"user_id": {strings.Split(c.token, ":")[0]},
	}

	var result struct {
		Ok          bool       `json:"ok"`
		Description string     `json:"description,omitempty"`
		Result      ChatMember `json:"result"`
	}

	if err := c.makeRequest("GET", endpoint, params, &result); err != nil {
		c.logger.Printf("Failed to get bot membership info: %v", err)
		return nil, fmt.Errorf("failed to get bot membership info: %w", err)
	}

	c.logger.Printf("Got bot membership info for chat %s: status=%s, can_invite_users=%v",
		chatID, result.Result.Status, result.Result.CanInviteUsers)

	return &result.Result, nil
}

func (c *Client) checkChatMembership(ctx context.Context, userID, chatID int64) (bool, error) {
	c.logger.Printf("Checking membership for user %d in chat %d", userID, chatID)

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", c.token)
	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"user_id": {fmt.Sprintf("%d", userID)},
	}

	var result struct {
		Ok     bool       `json:"ok"`
		Result ChatMember `json:"result"`
	}

	if err := c.makeRequest("GET", endpoint, params, &result); err != nil {
		// Проверяем на ошибку RPS
		if strings.Contains(err.Error(), "429") {
			return false, &RPSError{Description: "too many requests"}
		}
		return false, fmt.Errorf("failed to check membership: %w", err)
	}

	isMember := result.Result.Status == "member" ||
		result.Result.Status == "administrator" ||
		result.Result.Status == "creator"

	c.logger.Printf("Membership check result for user %d in chat %d: %v (status: %s)",
		userID, chatID, isMember, result.Result.Status)

	return isMember, nil
}

func (c *Client) checkBoostLevel(ctx context.Context, userID, chatID int64) (int, error) {
	c.logger.Printf("Checking boost level for user %d in chat %d", userID, chatID)

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChatBoostStatus", c.token)
	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"user_id": {fmt.Sprintf("%d", userID)},
	}

	var result struct {
		Ok     bool `json:"ok"`
		Result struct {
			Level int `json:"boost_level"`
		} `json:"result"`
	}

	if err := c.makeRequest("GET", endpoint, params, &result); err != nil {
		// Проверяем на ошибку RPS
		if strings.Contains(err.Error(), "429") {
			return 0, &RPSError{Description: "too many requests"}
		}
		return 0, fmt.Errorf("failed to check boost level: %w", err)
	}

	c.logger.Printf("Boost level for user %d in chat %d: %d",
		userID, chatID, result.Result.Level)

	return result.Result.Level, nil
}

func (c *Client) sendMessage(chatID int64, text string) (*apiResponse, error) {
	c.logger.Printf("Sending message to chat %d", chatID)

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.token)
	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"text":    {text},
	}

	var response apiResponse
	err := c.makeRequest("POST", endpoint, params, &response)
	if err != nil {
		c.logger.Printf("Failed to send message to chat %d: %v", chatID, err)
		return nil, err
	}

	c.logger.Printf("Successfully sent message to chat %d", chatID)
	return &response, nil
}

func (c *Client) makeRequest(method, endpoint string, params url.Values, result interface{}) error {
	c.logger.Printf("Making %s request to %s", method, endpoint)

	var req *http.Request
	var err error

	if method == "GET" && params != nil {
		endpoint = fmt.Sprintf("%s?%s", endpoint, params.Encode())
		req, err = http.NewRequest(method, endpoint, nil)
	} else {
		req, err = http.NewRequest(method, endpoint, strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if err != nil {
		c.logger.Printf("Failed to create request: %v", err)
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Printf("Failed to make request: %v", err)
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Printf("Failed to read response body: %v", err)
		return fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Printf("Response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		c.logger.Printf("Telegram API returned non-200 status code: %d, body: %s", resp.StatusCode, string(body))
		return fmt.Errorf("telegram API returned non-200 status code: %d, body: %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, result); err != nil {
		c.logger.Printf("Failed to unmarshal response: %v", err)
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

// GetChatIDByUsername получает числовой ID канала по его юзернейму
func (c *Client) GetChatIDByUsername(username string) (int64, error) {
	if !strings.HasPrefix(username, "@") {
		username = "@" + username
	}

	c.logger.Printf("Getting chat ID for username %s", username)

	chat, err := c.GetChat(username)
	if err != nil {
		return 0, fmt.Errorf("failed to get chat info for %s: %w", username, err)
	}

	c.logger.Printf("Got chat ID %d for username %s", chat.ID, username)
	return chat.ID, nil
}
