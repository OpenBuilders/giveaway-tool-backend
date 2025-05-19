package models

import (
	"fmt"
	"strconv"
	"strings"
)

// RequirementType определяет тип требования для участия
type RequirementType string

const (
	RequirementTypeSubscription RequirementType = "subscription" // Подписка на канал/чат
	RequirementTypeBoost        RequirementType = "boost"
)

// ChatType определяет тип чата
type ChatType string

const (
	ChatTypeChannel    ChatType = "channel"
	ChatTypeGroup      ChatType = "group"
	ChatTypeSupergroup ChatType = "supergroup"
)

// Requirement представляет требование для участия в розыгрыше
type Requirement struct {
	Type      RequirementType `json:"type"`
	ChatID    string          `json:"chat_id"`             // ID чата в формате строки (например, "@channel" или "-100123456789")
	ChatTitle string          `json:"chat_title"`          // Название чата для отображения
	MinLevel  int             `json:"min_level,omitempty"` // Для бустов
}

// Requirements представляет все требования для участия в розыгрыше
type Requirements struct {
	Enabled      bool          `json:"enabled"`      // Включены ли требования
	Requirements []Requirement `json:"requirements"` // Список требований
	JoinType     string        `json:"join_type"`    // "all" - все требования, "any" - любое из требований
}

// GetChatID преобразует строковый ID чата в числовой формат
func (r *Requirement) GetChatID() (int64, error) {
	// Если ID начинается с "@", это юзернейм канала
	if strings.HasPrefix(r.ChatID, "@") {
		return 0, nil // В этом случае нужно будет использовать API Telegram для получения реального ID
	}

	// Пытаемся преобразовать строку в число
	return strconv.ParseInt(r.ChatID, 10, 64)
}

// Validate проверяет корректность требований
func (r *Requirements) Validate() []error {
	if !r.Enabled {
		return nil
	}

	var errors []error
	if len(r.Requirements) == 0 {
		errors = append(errors, fmt.Errorf("requirements list cannot be empty when enabled"))
		return errors
	}

	if r.JoinType != "all" && r.JoinType != "any" {
		errors = append(errors, fmt.Errorf("join_type must be either 'all' or 'any'"))
	}

	for _, req := range r.Requirements {
		switch req.Type {
		case RequirementTypeSubscription:
			if req.ChatID == "" {
				errors = append(errors, fmt.Errorf("chat_id is required for subscription requirement"))
			}
		case RequirementTypeBoost:
			if req.ChatID == "" {
				errors = append(errors, fmt.Errorf("chat_id is required for boost requirement"))
			}
			if req.MinLevel <= 0 {
				errors = append(errors, fmt.Errorf("min_level must be greater than 0 for boost requirement"))
			}
		default:
			errors = append(errors, fmt.Errorf("unknown requirement type: %s", req.Type))
		}
	}

	return errors
}
