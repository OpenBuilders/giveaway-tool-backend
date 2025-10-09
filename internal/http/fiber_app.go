package http

import (
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

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

	// CORS for frontends
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowedOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Telegram-Init-Data",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	}))

	// Public health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// User domain deps
	repo := pgrepo.NewUserRepository(pg)
	cache := rcache.NewUserCache(rdb, 5*time.Second)
	us := usersvc.NewService(repo, cache)
	chs := channels.NewService(rdb)
	uh := NewUserHandlersFiber(us, chs)

	// Giveaway domain deps
	gRepo := pgrepo.NewGiveawayRepository(pg)
	tgClient := telegram.NewClientFromEnv()
	gs := gsvc.NewService(gRepo).WithTelegram(tgClient)
	gh := NewGiveawayHandlersFiber(gs, chs, tgClient)

	// API groups
	ttl := time.Duration(cfg.InitDataTTL) * time.Second
	api := app.Group("/api")
	v1 := api.Group("/v1", mw.RedisCache(rdb, 2*time.Second), mw.InitDataMiddleware(cfg.TelegramBotToken, ttl))
	uh.RegisterFiber(v1)
	gh.RegisterFiber(v1)

	// Telegram channels endpoints (public; no init-data required)
	ch := NewChannelHandlers(tgClient)
	ch.RegisterFiber(v1)
	rq := NewRequirementsHandlers(tgClient)
	rq.RegisterFiber(v1)

	return app
}
