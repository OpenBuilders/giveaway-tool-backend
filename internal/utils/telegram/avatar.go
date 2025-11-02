package telegram

import "fmt"

// BuildAvatarURL constructs the Telegram avatar URL for a given username.
// Returns an empty string if username is empty.
func BuildAvatarURL(username string) string {
	if username == "" {
		return ""
	}
	// return fmt.Sprintf("https://t.me/i/userpic/160/%s.jpg", username)
	return fmt.Sprintf("http://localhost:8080/api/public/channels/%s/avatar", username)
}
