package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	initdata "github.com/telegram-mini-apps/init-data-golang"
)

// Context keys to store Telegram init-data derived fields.
const (
	UserIdCtxParam       = "user_id"
	FirstNameCtxParam    = "first_name"
	LastNameCtxParam     = "last_name"
	UsernameCtxParam     = "username"
	UserPicCtxParam      = "photo_url"
	IsPremiumCtxParam    = "is_premium"
	LanguageCodeCtxParam = "language_code"
)

// InitDataMiddleware validates Telegram Mini Apps init-data and stores parsed fields in context.
// It expects init-data in one of the following places (checked in order):
//  1. Header: "X-Telegram-Init-Data"
//  2. Query:  "init_data" (raw string)
//
// If token is empty, the middleware will return 500 to avoid insecure defaults.
func InitDataMiddleware(token string, expIn time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Allow public health endpoint without validation
		if c.Path() == "/health" {
			return c.Next()
		}

		if token == "" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "init-data validation is not configured"})
		}

		initData := c.Get("X-Telegram-Init-Data")
		if initData == "" {
			initData = c.Query("init_data")
		}
		if initData == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing init_data"})
		}

		// Validate signature and expiration (expIn==0 disables TTL check as per library contract)
		if err := initdata.Validate(initData, token, expIn); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid init_data"})
		}

		// Parse to extract fields
		parsed, err := initdata.Parse(initData)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid init_data format"})
		}

		// parsed.User is a value type in the library; check by ID or other fields
		if parsed.User.ID != 0 {
			c.Locals(UserIdCtxParam, parsed.User.ID)
			c.Locals(FirstNameCtxParam, parsed.User.FirstName)
			c.Locals(LastNameCtxParam, parsed.User.LastName)
			c.Locals(UsernameCtxParam, parsed.User.Username)
			c.Locals(UserPicCtxParam, parsed.User.PhotoURL)
			c.Locals(IsPremiumCtxParam, parsed.User.IsPremium)
			c.Locals(LanguageCodeCtxParam, parsed.User.LanguageCode)
		}

		return c.Next()
	}
}
