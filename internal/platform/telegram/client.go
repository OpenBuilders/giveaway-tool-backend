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
	for _, req := range requirements {
		// Получаем числовой ID чата
		chatID, err := req.GetChatID()
		if err != nil {
			return nil, fmt.Errorf("invalid chat_id format: %w", err)
		}

		// Если получили юзернейм канала, пытаемся получить его ID через API
		if chatID == 0 {
			chat, err := c.GetChat(req.ChatID)
			if err != nil {
				return nil, fmt.Errorf("failed to get chat info: %w", err)
			}
			chatID = chat.ID
		}

		// Проверяем права бота в чате
		chatMember, err := c.GetBotChatMember(req.ChatID)
		if err != nil {
			return nil, fmt.Errorf("failed to get bot chat member: %w", err)
		}

		if !chatMember.CanInviteUsers {
			errors = append(errors, fmt.Errorf("bot needs invite users permission in chat %s", req.ChatID))
		}
	}

	return errors, nil
}

// CheckRequirements проверяет выполнение требований пользователем
func (c *Client) CheckRequirements(ctx context.Context, userID int64, requirements *models.Requirements) (bool, error) {
	for _, req := range requirements.Requirements {
		// Получаем числовой ID чата
		chatID, err := req.GetChatID()
		if err != nil {
			return false, fmt.Errorf("invalid chat_id format: %w", err)
		}

		// Если получили юзернейм канала, пытаемся получить его ID через API
		if chatID == 0 {
			chat, err := c.GetChat(req.ChatID)
			if err != nil {
				return false, fmt.Errorf("failed to get chat info: %w", err)
			}
			chatID = chat.ID
		}

		switch req.Type {
		case models.RequirementTypeSubscription:
			isMember, err := c.checkChatMembership(ctx, userID, chatID)
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

		// case models.RequirementTypeBoost:
		// 	level, err := c.checkBoostLevel(ctx, userID, chatID)
		// 	if err != nil {
		// 		// Если получили ошибку RPS, пропускаем проверку
		// 		var rpsErr *RPSError
		// 		if ok := errors.As(err, &rpsErr); ok {
		// 			return true, nil
		// 		}
		// 		return false, err
		// 	}
		// 	if level < req.MinLevel {
		// 		return false, nil
		// 	}
		// }
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

	err := c.sendMessage(userID, message)
	if err != nil {
		c.logger.Printf("Failed to send notification to winner %d: %v", userID, err)
		return err
	}

	c.logger.Printf("Successfully sent notification to winner %d", userID)
	return nil
}

func (c *Client) NotifyCreator(userID int64, giveaway *models.Giveaway) error {
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

	err := c.sendMessage(userID, message)
	if err != nil {
		c.logger.Printf("Failed to send notification to creator %d: %v", userID, err)
		return err
	}

	c.logger.Printf("Successfully sent notification to creator %d", userID)
	return nil
}

// Вспомогательные методы

// GetChat retrieves information about a chat by username or ID
func (c *Client) GetChat(chatID string) (*Chat, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChat", c.token)
	params := url.Values{
		"chat_id": {chatID},
	}

	var result struct {
		Ok     bool `json:"ok"`
		Result Chat `json:"result"`
	}

	if err := c.makeRequest("GET", endpoint, params, &result); err != nil {
		return nil, err
	}

	return &result.Result, nil
}

// GetBotChatMember checks if the bot is a member of the specified chat and returns its status
func (c *Client) GetBotChatMember(chatID string) (*ChatMember, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember", c.token)
	params := url.Values{
		"chat_id": {chatID},
		"user_id": {strings.Split(c.token, ":")[0]},
	}

	c.logger.Printf("Getting bot membership info for chat %s", chatID)

	resp, err := http.PostForm(endpoint, params)
	if err != nil {
		c.logger.Printf("Failed to get bot membership info: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	c.logger.Printf("Received response with status code: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		c.logger.Printf("Failed to get bot membership info. Status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("failed to get bot membership info. Status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Printf("Failed to read response body: %v", err)
		return nil, err
	}

	c.logger.Printf("Response body: %s", string(body))

	var result struct {
		Ok     bool       `json:"ok"`
		Result ChatMember `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		c.logger.Printf("Failed to unmarshal response: %v", err)
		return nil, err
	}

	if !result.Ok {
		return nil, fmt.Errorf("telegram API error")
	}

	return &result.Result, nil
}

func (c *Client) checkChatMembership(ctx context.Context, userID, chatID int64) (bool, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember?chat_id=%d&user_id=%d", c.token, chatID, userID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return false, &RPSError{Description: "too many requests"}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var result struct {
		Ok     bool       `json:"ok"`
		Result ChatMember `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	if !result.Ok {
		return false, fmt.Errorf("telegram API error")
	}

	return result.Result.Status == "member" || result.Result.Status == "administrator" || result.Result.Status == "creator", nil
}

func (c *Client) checkBoostLevel(ctx context.Context, userID, chatID int64) (int, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getChatBoostStatus?chat_id=%d&user_id=%d", c.token, chatID, userID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, &RPSError{Description: "too many requests"}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Ok     bool `json:"ok"`
		Result struct {
			Level int `json:"boost_level"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if !result.Ok {
		return 0, fmt.Errorf("telegram API error")
	}

	return result.Result.Level, nil
}

func (c *Client) sendMessage(chatID int64, text string) error {
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
		return err
	}

	c.logger.Printf("Successfully sent message to chat %d", chatID)
	return nil
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
