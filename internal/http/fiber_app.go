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
	"github.com/your-org/giveaway-backend/internal/service/channels"
	gsvc "github.com/your-org/giveaway-backend/internal/service/giveaway"
	"github.com/your-org/giveaway-backend/internal/service/telegram"
	usersvc "github.com/your-org/giveaway-backend/internal/service/user"
)

// NewFiberApp builds a Fiber application with routes and middlewares wired.
func NewFiberApp(pg *sql.DB, rdb *redisp.Client, cfg *config.Config) *fiber.App {
	app := fiber.New()

	// Public health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// User domain deps
	repo := pgrepo.NewUserRepository(pg)
	cache := rcache.NewUserCache(rdb, 5*time.Minute)
	us := usersvc.NewService(repo, cache)
	chs := channels.NewService(rdb)
	uh := NewUserHandlersFiber(us, chs)

	// Giveaway domain deps
	gRepo := pgrepo.NewGiveawayRepository(pg)
	gs := gsvc.NewService(gRepo)
	gh := NewGiveawayHandlersFiber(gs)

	// API group with Telegram init-data middleware
	ttl := time.Duration(cfg.InitDataTTL) * time.Second
	api := app.Group("/api", mw.InitDataMiddleware(cfg.TelegramBotToken, ttl))
	uh.RegisterFiber(api)
	gh.RegisterFiber(api)

	// Telegram channels endpoints (public; no init-data required)
	tg := telegram.NewClientFromEnv()
	ch := NewChannelHandlers(tg)
	ch.RegisterFiber(app.Group("/api"))

	return app
}
