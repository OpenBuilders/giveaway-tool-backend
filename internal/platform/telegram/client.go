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
	"strconv"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
	token      string
	logger     *log.Logger
}

// RPSError представляет ошибку превышения лимита запросов
type RPSError struct {
	Msg string
}

func (e *RPSError) Error() string {
	return e.Msg
}

// ChatMember представляет информацию о пользователе в чате
type ChatMember struct {
	Status             string `json:"status"`
	CanInviteUsers     bool   `json:"can_invite_users"`
	CanRestrictMembers bool   `json:"can_restrict_members"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

// Response представляет ответ от Telegram API
type Response struct {
	Ok          bool        `json:"ok"`
	Result      interface{} `json:"result,omitempty"`
	Error       string      `json:"error,omitempty"`
	Description string      `json:"description,omitempty"`
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
func (c *Client) ValidateRequirements(requirements *models.Requirements) ([]string, error) {
	var errors []string

	// Если требования отключены или пусты, возвращаем пустой список ошибок
	if requirements == nil || len(requirements.Requirements) == 0 {
		return errors, nil
	}

	// Проверяем каждое требование
	for _, req := range requirements.Requirements {
		// Проверяем доступность бота в чате
		chat, err := c.GetChat(req.Username)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to get chat %s: %v", req.Username, err))
			continue
		}

		// Проверяем права бота в чате
		member, err := c.GetBotChatMember(fmt.Sprintf("%d", chat.ID))
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to get bot member info for chat %s: %v", req.Username, err))
			continue
		}

		// Проверяем тип требования
		switch req.Type {
		case models.RequirementTypeSubscription:
			// Для подписки достаточно базовых прав
			if !member.CanInviteUsers {
				errors = append(errors, fmt.Sprintf("bot doesn't have enough rights in chat %s to check subscriptions", req.Username))
			}

		case models.RequirementTypeBoost:
			// Для проверки бустов нужны права администратора
			if !member.CanInviteUsers || !member.CanRestrictMembers {
				errors = append(errors, fmt.Sprintf("bot doesn't have enough rights in chat %s to check boosts", req.Username))
			}

		default:
			errors = append(errors, fmt.Sprintf("unknown requirement type: %s", req.Type))
		}
	}

	return errors, nil
}

// CheckRequirements проверяет выполнение требований пользователем
func (c *Client) CheckRequirements(ctx context.Context, userID int64, requirements *models.Requirements) (bool, error) {
	for _, req := range requirements.Requirements {
		switch req.Type {
		case models.RequirementTypeSubscription:
			isMember, err := c.CheckMembership(ctx, userID, req.Username)
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

		case models.RequirementTypeBoost:
			hasBoost, err := c.CheckBoost(ctx, userID, req.Username)
			if err != nil {
				// Если получили ошибку RPS, пропускаем проверку
				var rpsErr *RPSError
				if ok := errors.As(err, &rpsErr); ok {
					return true, nil
				}
				return false, err
			}
			if !hasBoost {
				return false, nil
			}

		default:
			return false, fmt.Errorf("unknown requirement type: %s", req.Type)
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

func (c *Client) NotifyCreator(userID int64, giveaway *models.Giveaway) error {
	c.logger.Printf("Sending notification to creator %d for giveaway %s", userID, giveaway.ID)

	message := fmt.Sprintf(
		"✨ Розыгрыш успешно создан!\\n\\n"+
			"📋 Название: %s\\n"+
			"📝 Описание: %s\\n"+
			"⏰ Длительность: %d секунд\\n"+
			"👥 Количество победителей: %d\\n\\n"+
			"🎯 Статус: %s",
		giveaway.Title,
		giveaway.Description,
		giveaway.Duration,
		giveaway.WinnersCount,
		giveaway.Status,
	)

	if giveaway.MaxParticipants > 0 {
		message += fmt.Sprintf("\\n👥 Максимум участников: %d", giveaway.MaxParticipants)
	}

	response, err := c.sendMessage(userID, message)
	if err != nil {
		c.logger.Printf("Failed to send notification to creator %d: %v", userID, err)
		return err
	}

	if !response.Ok {
		return fmt.Errorf("telegram API error: %s", response.Error)
	}

	c.logger.Printf("Successfully sent notification to creator %d", userID)
	return nil
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
			return false, &RPSError{Msg: "too many requests"}
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
			return 0, &RPSError{Msg: "too many requests"}
		}
		return 0, fmt.Errorf("failed to check boost level: %w", err)
	}

	c.logger.Printf("Boost level for user %d in chat %d: %d",
		userID, chatID, result.Result.Level)

	return result.Result.Level, nil
}

func (c *Client) sendMessage(chatID int64, text string) (*Response, error) {
	c.logger.Printf("Sending message to chat %d", chatID)

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.token)
	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"text":    {text},
	}

	var response Response
	if err := c.makeRequest("POST", endpoint, params, &response); err != nil {
		c.logger.Printf("Failed to send message to chat %d: %v", chatID, err)
		return nil, err
	}

	if !response.Ok {
		return nil, fmt.Errorf("telegram API error: %s", response.Error)
	}

	c.logger.Printf("Successfully sent message to chat %d", chatID)
	return &response, nil
}

// makeRequest отправляет запрос к Telegram API и возвращает результат
func (c *Client) makeRequest(method, endpoint string, data url.Values, result interface{}) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	var req *http.Request
	var err error

	if method == "POST" {
		req, err = http.NewRequest(method, endpoint, strings.NewReader(data.Encode()))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		if len(data) > 0 {
			endpoint = fmt.Sprintf("%s?%s", endpoint, data.Encode())
		}
		req, err = http.NewRequest(method, endpoint, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
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

// CheckMembership проверяет, является ли пользователь участником канала/чата
func (c *Client) CheckMembership(ctx context.Context, userID int64, chatID string) (bool, error) {
	// Преобразуем chatID в числовой формат, если это возможно
	var numericChatID int64
	if chatID[0] == '@' {
		// Если это юзернейм, сначала получаем информацию о чате
		chat, err := c.GetChat(chatID)
		if err != nil {
			return false, fmt.Errorf("failed to get chat info: %w", err)
		}
		numericChatID = chat.ID
	} else {
		// Пробуем преобразовать строку в число
		var err error
		numericChatID, err = strconv.ParseInt(chatID, 10, 64)
		if err != nil {
			return false, fmt.Errorf("invalid chat ID format: %w", err)
		}
	}

	// Формируем запрос к Telegram API
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

	if err := c.makeRequest("GET", endpoint, data, &response); err != nil {
		return false, fmt.Errorf("failed to check membership: %w", err)
	}

	if !response.Ok {
		if strings.Contains(response.Error, "Too Many Requests") {
			return false, &RPSError{Msg: "Rate limit exceeded"}
		}
		return false, fmt.Errorf("telegram API error: %s", response.Error)
	}

	// Считаем пользователя участником, если его статус один из следующих:
	validStatuses := []string{"creator", "administrator", "member", "restricted"}
	for _, validStatus := range validStatuses {
		if response.Result.Status == validStatus {
			return true, nil
		}
	}

	return false, nil
}

// CheckBoost проверяет, бустит ли пользователь канал
func (c *Client) CheckBoost(ctx context.Context, userID int64, chatID string) (bool, error) {
	// Преобразуем chatID в числовой формат, если это возможно
	var numericChatID int64
	if chatID[0] == '@' {
		// Если это юзернейм, сначала получаем информацию о чате
		chat, err := c.GetChat(chatID)
		if err != nil {
			return false, fmt.Errorf("failed to get chat info: %w", err)
		}
		numericChatID = chat.ID
	} else {
		// Пробуем преобразовать строку в число
		var err error
		numericChatID, err = strconv.ParseInt(chatID, 10, 64)
		if err != nil {
			return false, fmt.Errorf("invalid chat ID format: %w", err)
		}
	}

	// Формируем запрос к Telegram API
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUserChatBoosts", c.token)
	data := url.Values{
		"chat_id": {fmt.Sprintf("%d", numericChatID)},
		"user_id": {fmt.Sprintf("%d", userID)},
	}

	var response struct {
		Ok     bool   `json:"ok"`
		Error  string `json:"error"`
		Result struct {
			Boosts []interface{} `json:"boosts"`
		} `json:"result"`
	}

	if err := c.makeRequest("GET", endpoint, data, &response); err != nil {
		return false, fmt.Errorf("failed to check boost status: %w", err)
	}

	if !response.Ok {
		if strings.Contains(response.Error, "Too Many Requests") {
			return false, &RPSError{Msg: "Rate limit exceeded"}
		}
		return false, fmt.Errorf("telegram API error: %s", response.Error)
	}

	// Если есть хотя бы один активный буст, возвращаем true
	return len(response.Result.Boosts) > 0, nil
}
