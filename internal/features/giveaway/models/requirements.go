package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	RequirementTypeSubscription = "subscription"
	RequirementTypeBoost        = "boost"
	RequirementTypeCustom       = "custom"
)

const (
	RequirementNameTemplateSubscription = "Follow %s"
	RequirementNameTemplateBoost        = "Boost %s"
	RequirementNameTemplateCustom       = "Custom: %s"
)

const (
	RequirementStatusPending = "pending"
	RequirementStatusSuccess = "success"
	RequirementStatusFailed  = "failed"
	RequirementStatusSkipped = "skipped"
	RequirementStatusError   = "error"
)

type Requirement struct {
	Type        string `json:"type"`
	Username    string `json:"username"`
	Description string `json:"description"`
	name        string
}

func (r *Requirement) Name() string {
	if r.name != "" {
		return r.name
	}

	switch r.Type {
	case RequirementTypeSubscription:
		username := r.Username
		if !strings.HasPrefix(username, "@") {
			username = "@" + username
		}
		r.name = fmt.Sprintf(RequirementNameTemplateSubscription, username)
	case RequirementTypeBoost:
		username := r.Username
		if !strings.HasPrefix(username, "@") {
			username = "@" + username
		}
		r.name = fmt.Sprintf(RequirementNameTemplateBoost, username)
	case RequirementTypeCustom:
		description := r.Description
		if description == "" {
			description = "Custom requirement"
		}
		r.name = fmt.Sprintf(RequirementNameTemplateCustom, description)
	default:
		r.name = fmt.Sprintf("Unknown requirement: %s", r.Username)
	}

	return r.name
}

func (r *Requirement) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Username    string `json:"username"`
		Description string `json:"description,omitempty"`
	}{
		Name:        r.Name(),
		Type:        r.Type,
		Username:    r.Username,
		Description: r.Description,
	})
}

func (r *Requirement) UnmarshalJSON(data []byte) error {
	aux := struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Username    string `json:"username"`
		Description string `json:"description"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.Type = aux.Type
	r.Username = aux.Username
	r.Description = aux.Description
	return nil
}

func (r *Requirement) Validate() error {
	if r.Type != RequirementTypeSubscription && r.Type != RequirementTypeBoost && r.Type != RequirementTypeCustom {
		return fmt.Errorf("invalid requirement type: %s", r.Type)
	}

	switch r.Type {
	case RequirementTypeSubscription, RequirementTypeBoost:
		if r.Username == "" {
			return fmt.Errorf("username is required for %s requirement", r.Type)
		}
		if !strings.HasPrefix(r.Username, "@") {
			r.Username = "@" + r.Username
		}
	case RequirementTypeCustom:
		if r.Description == "" {
			return fmt.Errorf("description is required for custom requirement")
		}
	}

	return nil
}

func (r *Requirement) IsCustom() bool {
	return r.Type == RequirementTypeCustom
}

type ChatInfo struct {
	Title     string `json:"title"`
	Username  string `json:"username"`
	Type      string `json:"type"`
	AvatarURL string `json:"avatar_url"`
}

type RequirementCheckResult struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Username string    `json:"username"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	ChatInfo *ChatInfo `json:"chat_info,omitempty"`
}

type RequirementsCheckResponse struct {
	GiveawayID string                   `json:"giveaway_id"`
	Results    []RequirementCheckResult `json:"results"`
	AllMet     bool                     `json:"all_met"`
}

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
	if t.Type != RequirementTypeSubscription && t.Type != RequirementTypeBoost && t.Type != RequirementTypeCustom {
		return fmt.Errorf("invalid requirement type: %s", t.Type)
	}
	return nil
}

type ChatIDInfo struct {
	RawID     string
	Username  string
	IsNumeric bool
	NumericID int64
}

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

func (r *Requirements) HasCustomRequirements() bool {
	for _, req := range r.Requirements {
		if req.IsCustom() {
			return true
		}
	}
	return false
}

func (r *Requirements) GetNonCustomRequirements() []Requirement {
	var nonCustom []Requirement
	for _, req := range r.Requirements {
		if !req.IsCustom() {
			nonCustom = append(nonCustom, req)
		}
	}
	return nonCustom
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
