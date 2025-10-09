package giveaway

// RequirementType enumerates allowed requirement kinds.
type RequirementType string

const (
	RequirementTypeSubscription RequirementType = "subscription"
)

// Requirement describes a single requirement entry for a giveaway.
// For subscription, either ChannelID or ChannelUsername should be provided.
type Requirement struct {
	Type            RequirementType `json:"type"`
	ChannelID       int64           `json:"channel_id,omitempty"`
	ChannelUsername string          `json:"channel_username,omitempty"`
	ChannelTitle    string          `json:"channel_title,omitempty"`
	ChannelURL      string          `json:"channel_url,omitempty"`
	AvatarURL       string          `json:"avatar_url,omitempty"`
}
