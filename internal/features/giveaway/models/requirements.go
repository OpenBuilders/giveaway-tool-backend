package models

import (
	"fmt"
	"strconv"
	"strings"
)

// Requirement types
const (
	RequirementTypeSubscription = "subscription"
)

type RequirementTemplate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func (t *RequirementTemplate) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("requirement template ID is required")
	}
	if t.Name == "" {
		return fmt.Errorf("requirement template name is required")
	}
	if t.Type != RequirementTypeSubscription {
		return fmt.Errorf("invalid requirement type: %s", t.Type)
	}
	return nil
}

func ValidateTemplates(templates []RequirementTemplate) error {
	if len(templates) == 0 {
		return fmt.Errorf("at least one requirement template is required")
	}
	for _, template := range templates {
		if err := template.Validate(); err != nil {
			return fmt.Errorf("invalid requirement template: %w", err)
		}
	}
	return nil
}

type Requirement struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Type   string `json:"type"`
	ChatID string `json:"chat_id"`
}

func (r *Requirement) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("requirement name is required")
	}
	if r.Type != RequirementTypeSubscription {
		return fmt.Errorf("invalid requirement type: %s", r.Type)
	}
	if r.Value == "" {
		return fmt.Errorf("requirement value is required")
	}
	if r.ChatID == "" {
		return fmt.Errorf("chat_id is required")
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
		RawID: r.ChatID,
	}

	if strings.HasPrefix(r.ChatID, "@") {
		info.Username = strings.TrimPrefix(r.ChatID, "@")
		info.IsNumeric = false
		return info, nil
	}

	chatID, err := strconv.ParseInt(r.ChatID, 10, 64)
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
