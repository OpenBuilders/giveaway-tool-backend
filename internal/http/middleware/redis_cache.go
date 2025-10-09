package middleware

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	rplatform "github.com/your-org/giveaway-backend/internal/platform/redis"
)

type cachedResponse struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

// RedisCache caches GET responses for a short TTL. Keyed by method+full URL.
func RedisCache(rdb *rplatform.Client, ttl time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() != fiber.MethodGet {
			return c.Next()
		}

		key := "httpcache:" + c.Method() + ":" + string(c.Request().URI().FullURI())

		if bs, err := rdb.Get(c.Context(), key).Bytes(); err == nil && len(bs) > 0 {
			var entry cachedResponse
			if json.Unmarshal(bs, &entry) == nil {
				if entry.ContentType != "" {
					c.Set(fiber.HeaderContentType, entry.ContentType)
				}
				c.Set("X-Cache", "HIT")
				c.Status(entry.Status)
				return c.Send(entry.Body)
			}
		}

		if err := c.Next(); err != nil {
			return err
		}

		status := c.Response().StatusCode()
		if status >= 200 && status < 300 {
			ct := string(c.Response().Header.ContentType())
			body := c.Response().Body()
			entry := cachedResponse{Status: status, ContentType: ct, Body: append([]byte(nil), body...)}
			if payload, err := json.Marshal(entry); err == nil {
				_ = rdb.SetEx(context.Background(), key, payload, ttl).Err()
			}
		}
		c.Set("X-Cache", "MISS")
		return nil
	}
}
