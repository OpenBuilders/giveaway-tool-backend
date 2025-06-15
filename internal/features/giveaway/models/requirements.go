package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Типы требований
const (
	RequirementTypeSubscription = "subscription" // Подписка на канал
	RequirementTypeBoost        = "boost"        // Буст канала
)

// Шаблоны названий требований
const (
	RequirementNameTemplateSubscription = "Follow %s" // Шаблон для подписки
	RequirementNameTemplateBoost        = "Boost %s"  // Шаблон для буста
)

// Статусы проверки требований
const (
	RequirementStatusPending = "pending" // Ожидает проверки
	RequirementStatusSuccess = "success" // Успешно выполнено
	RequirementStatusFailed  = "failed"  // Не выполнено
	RequirementStatusSkipped = "skipped" // Пропущено (например, из-за RPS)
	RequirementStatusError   = "error"   // Ошибка при проверке
)

// Requirement представляет требование для участия в розыгрыше
type Requirement struct {
	Type     string `json:"type"`     // Тип требования (subscription, boost)
	Username string `json:"username"` // Username канала (с @ или без)
	name     string // Внутреннее поле для кэширования сгенерированного имени
}

// Name возвращает сгенерированное имя требования
func (r *Requirement) Name() string {
	if r.name != "" {
		return r.name
	}

	username := r.Username
	if !strings.HasPrefix(username, "@") {
		username = "@" + username
	}

	switch r.Type {
	case RequirementTypeSubscription:
		r.name = fmt.Sprintf(RequirementNameTemplateSubscription, username)
	case RequirementTypeBoost:
		r.name = fmt.Sprintf(RequirementNameTemplateBoost, username)
	default:
		r.name = fmt.Sprintf("Unknown requirement for %s", username)
	}

	return r.name
}

// MarshalJSON реализует пользовательскую сериализацию в JSON
func (r *Requirement) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Username string `json:"username"`
	}{
		Name:     r.Name(),
		Type:     r.Type,
		Username: r.Username,
	})
}

// UnmarshalJSON реализует пользовательскую десериализацию из JSON
func (r *Requirement) UnmarshalJSON(data []byte) error {
	aux := struct {
		Name     string `json:"name"` // Игнорируем name при десериализации
		Type     string `json:"type"`
		Username string `json:"username"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.Type = aux.Type
	r.Username = aux.Username
	return nil
}

// Validate проверяет корректность требования
func (r *Requirement) Validate() error {
	if r.Type != RequirementTypeSubscription && r.Type != RequirementTypeBoost {
		return fmt.Errorf("invalid requirement type: %s", r.Type)
	}
	if r.Username == "" {
		return fmt.Errorf("username is required")
	}
	// Добавляем @ к username, если его нет
	if !strings.HasPrefix(r.Username, "@") {
		r.Username = "@" + r.Username
	}
	return nil
}

// ChatInfo содержит информацию о канале
type ChatInfo struct {
	Title     string `json:"title"`
	Username  string `json:"username"`
	Type      string `json:"type"`
	AvatarURL string `json:"avatar_url"`
}

// RequirementCheckResult содержит результат проверки требования
type RequirementCheckResult struct {
	Name     string    `json:"name"`                // Название требования
	Type     string    `json:"type"`                // Тип требования
	Username string    `json:"username"`            // Username канала
	Status   string    `json:"status"`              // Статус проверки
	Error    string    `json:"error,omitempty"`     // Ошибка, если есть
	ChatInfo *ChatInfo `json:"chat_info,omitempty"` // Информация о канале
}

// RequirementsCheckResponse содержит результаты проверки всех требований
type RequirementsCheckResponse struct {
	GiveawayID string                   `json:"giveaway_id"` // ID розыгрыша
	Results    []RequirementCheckResult `json:"results"`     // Результаты проверки
	AllMet     bool                     `json:"all_met"`     // Все требования выполнены
}

// RequirementTemplate представляет шаблон требования
type RequirementTemplate struct {
	ID   string `json:"id"`   // Уникальный идентификатор шаблона
	Name string `json:"name"` // Название для отображения
	Type string `json:"type"` // Тип требования
}

func (t *RequirementTemplate) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("requirement template ID is required")
	}
	if t.Name == "" {
		return fmt.Errorf("requirement template name is required")
	}
	if t.Type != RequirementTypeSubscription && t.Type != RequirementTypeBoost {
		return fmt.Errorf("invalid requirement type: %s", t.Type)
	}
	return nil
}

// ChatIDInfo содержит информацию об идентификаторе чата
type ChatIDInfo struct {
	RawID     string // Исходный ID как он был передан
	Username  string // Юзернейм чата (если есть)
	IsNumeric bool   // Флаг, указывающий является ли ID числовым
	NumericID int64  // Числовой ID чата
}

// GetChatIDInfo анализирует формат переданного идентификатора чата
// Важно: этот метод только парсит формат, но не делает API-запросы
// Для получения числового ID используйте telegram.Client.GetChatIDByUsername
func (r *Requirement) GetChatIDInfo() (*ChatIDInfo, error) {
	info := &ChatIDInfo{
		RawID: r.Username,
	}

	if strings.HasPrefix(r.Username, "@") {
		info.Username = strings.TrimPrefix(r.Username, "@")
		info.IsNumeric = false
		return info, nil
	}

	chatID, err := strconv.ParseInt(r.Username, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat_id format: %w", err)
	}

	info.IsNumeric = true
	info.NumericID = chatID
	return info, nil
}

type Requirements struct {
	Requirements []Requirement `json:"requirements"`
	Enabled      bool          `json:"enabled"`
}

func (r *Requirements) Validate() error {
	if len(r.Requirements) == 0 {
		return fmt.Errorf("at least one requirement is required")
	}
	for _, req := range r.Requirements {
		if err := req.Validate(); err != nil {
			return fmt.Errorf("invalid requirement: %w", err)
		}
	}
	return nil
}

func ValidateRequirements(reqs []Requirement) error {
	if len(reqs) == 0 {
		return fmt.Errorf("at least one requirement is required")
	}
	for _, req := range reqs {
		if err := req.Validate(); err != nil {
			return fmt.Errorf("invalid requirement: %w", err)
		}
	}
	return nil
}
