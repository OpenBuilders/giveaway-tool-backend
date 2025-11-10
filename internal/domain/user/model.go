package user

import "time"

// User represents an application user mirrored from Telegram identity.
// ID is a Telegram user ID; we store profile fields for convenience.
type User struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	Role          string    `json:"role"`   // allowed: "user", "admin"
	Status        string    `json:"status"` // allowed: "active", "banned"
	WalletAddress string    `json:"wallet_address,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
