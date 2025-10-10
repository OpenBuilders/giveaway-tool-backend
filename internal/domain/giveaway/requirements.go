package giveaway

// RequirementType enumerates allowed requirement kinds.
type RequirementType string

const (
	RequirementTypeSubscription RequirementType = "subscription"
	RequirementTypeBoost        RequirementType = "boost"
	RequirementTypeCustom       RequirementType = "custom"
)

// Requirement describes a single requirement entry for a giveaway.
// For subscription, either ChannelID or ChannelUsername should be provided.
type Requirement struct {
	Type RequirementType `json:"type"`
	// Internal fields not exposed in API directly
	ChannelID  int64  `json:"-"`
	ChannelURL string `json:"-"`
	// API-facing fields per frontend contract
	ChannelUsername string `json:"username,omitempty"`
	ChannelTitle    string `json:"name,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	Description     string `json:"description,omitempty"`
}
