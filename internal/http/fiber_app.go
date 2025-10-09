package http

import (
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"

	rcache "github.com/your-org/giveaway-backend/internal/cache/redis"
	"github.com/your-org/giveaway-backend/internal/config"
	mw "github.com/your-org/giveaway-backend/internal/http/middleware"
	redisp "github.com/your-org/giveaway-backend/internal/platform/redis"
	pgrepo "github.com/your-org/giveaway-backend/internal/repository/postgres"
	"github.com/your-org/giveaway-backend/internal/service"
)

// NewFiberApp builds a Fiber application with routes and middlewares wired.
func NewFiberApp(pg *sql.DB, rdb *redisp.Client, cfg *config.Config) *fiber.App {
	app := fiber.New()

	// Public health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// Dependencies for user domain
	repo := pgrepo.NewUserRepository(pg)
	cache := rcache.NewUserCache(rdb, 5*time.Minute)
	svc := service.NewUserService(repo, cache, 5*time.Minute)
	uh := NewUserHandlersFiber(svc)

	// API group with Telegram init-data middleware
	ttl := time.Duration(cfg.InitDataTTL) * time.Second
	api := app.Group("/api", mw.InitDataMiddleware(cfg.TelegramBotToken, ttl))
	uh.RegisterFiber(api)

	return app
}
