package telegram

import (
	"fmt"
	"strings"

	"github.com/open-builders/giveaway-backend/internal/config"
)

// BuildAvatarURL constructs the public avatar URL using config's PublicBaseURL.
// Returns an empty string if username is empty.
func BuildAvatarURL(username string) string {
	if username == "" {
		return ""
	}
	baseURL := strings.TrimRight(config.GetPublicBaseURL(), "/")
	return fmt.Sprintf("%s/api/public/channels/%s/avatar", baseURL, username)
}
