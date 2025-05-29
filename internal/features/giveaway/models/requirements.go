package models

import (
	"fmt"
)

// RequirementTemplate представляет шаблон требования для розыгрыша
type RequirementTemplate struct {
	ID   string `json:"id"`   // Уникальный идентификатор шаблона
	Name string `json:"name"` // Название требования
	Type string `json:"type"` // Тип требования
}

// ValidateTemplate проверяет корректность шаблона требования
func (t *RequirementTemplate) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("requirement template ID is required")
	}
	if t.Name == "" {
		return fmt.Errorf("requirement template name is required")
	}
	if t.Type != "subscription" {
		return fmt.Errorf("invalid requirement type: %s", t.Type)
	}
	return nil
}

// ValidateTemplates проверяет корректность списка шаблонов требований
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

// Requirement представляет требование для участия в розыгрыше
type Requirements struct {
	Name  string      `json:"name"`  // Название требования
	Value interface{} `json:"value"` // Значение требования (может быть string или []string)
	Type  string      `json:"type"`  // Тип требования (например, "subscription")
}

// Validate проверяет корректность требования
func (r *Requirements) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("requirement name is required")
	}
	if r.Type != "subscription" {
		return fmt.Errorf("invalid requirement type: %s", r.Type)
	}
	if r.Value == nil {
		return fmt.Errorf("requirement value is required")
	}
	return nil
}

// ValidateRequirements проверяет корректность списка требований
func ValidateRequirements(reqs []Requirements) error {
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
