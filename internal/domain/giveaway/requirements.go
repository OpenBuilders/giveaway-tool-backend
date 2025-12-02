package giveaway

// RequirementType enumerates allowed requirement kinds.
type RequirementType string

const (
	RequirementTypeSubscription RequirementType = "subscription"
	RequirementTypeBoost        RequirementType = "boost"
	RequirementTypeCustom       RequirementType = "custom"
	RequirementTypePremium      RequirementType = "premium"
	// New on-chain requirements
	RequirementTypeHoldTON    RequirementType = "holdton"
	RequirementTypeHoldJetton RequirementType = "holdjetton"
	// New account age requirement
	RequirementTypeAccountAge RequirementType = "account_age"
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
	Title           string          `json:"title,omitempty"`
	Description     string          `json:"description,omitempty"`
	// On-chain checks
	// For holdton: required minimum TON balance in nanoTONs (1 TON = 1e9 nano)
	TonMinBalanceNano int64 `json:"ton_min_balance_nano,omitempty"`
	// For holdjetton: jetton master address and required minimum amount in smallest units
	JettonAddress   string `json:"jetton_address,omitempty"`
	JettonMinAmount int64  `json:"jetton_min_amount,omitempty"`
	// For account_age: maximum allowed registration year (inclusive)
	// E.g. if 2020, then 2020, 2019, 2018... are allowed.
	AccountAgeMaxYear int `json:"account_age_max_year,omitempty"`
}
