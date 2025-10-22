package http

import (
	"context"
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	rcache "github.com/open-builders/giveaway-backend/internal/cache/redis"
	"github.com/open-builders/giveaway-backend/internal/config"
	mw "github.com/open-builders/giveaway-backend/internal/http/middleware"
	redisp "github.com/open-builders/giveaway-backend/internal/platform/redis"
	pgrepo "github.com/open-builders/giveaway-backend/internal/repository/postgres"
	"github.com/open-builders/giveaway-backend/internal/service/channels"
	gsvc "github.com/open-builders/giveaway-backend/internal/service/giveaway"
	"github.com/open-builders/giveaway-backend/internal/service/telegram"
	"github.com/open-builders/giveaway-backend/internal/service/tonbalance"
	"github.com/open-builders/giveaway-backend/internal/service/tonproof"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
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

	// Liveness probe: process is up and Fiber is serving
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// Readiness probe: downstream deps (DB, Redis) are reachable
	app.Get("/readyz", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()

		deps := make(map[string]any)
		ready := true

		if err := pg.PingContext(ctx); err != nil {
			ready = false
			deps["postgres"] = fiber.Map{"ok": false, "error": err.Error()}
		} else {
			deps["postgres"] = fiber.Map{"ok": true}
		}

		if err := rdb.Ping(ctx).Err(); err != nil {
			ready = false
			deps["redis"] = fiber.Map{"ok": false, "error": err.Error()}
		} else {
			deps["redis"] = fiber.Map{"ok": true}
		}

		status := fiber.StatusOK
		if !ready {
			status = fiber.StatusServiceUnavailable
		}
		return c.Status(status).JSON(fiber.Map{"ready": ready, "deps": deps})
	})

	// User domain deps
	repo := pgrepo.NewUserRepository(pg)
	cache := rcache.NewUserCache(rdb, 5*time.Second)
	us := usersvc.NewService(repo, cache)
	chs := channels.NewService(rdb)
	uh := NewUserHandlersFiber(us, chs)
	// TON Proof service
	tps := tonproof.NewService(rdb, cfg.TonProofDomain, cfg.TonProofPayloadTTLSec, cfg.TonAPIBaseURL, cfg.TonAPIToken)
	uh.AttachTonProof(tps, cfg.TonProofDomain)

    // Giveaway domain deps
	gRepo := pgrepo.NewGiveawayRepository(pg)
	tgClient := telegram.NewClientFromEnv()
	gs := gsvc.NewService(gRepo).WithTelegram(tgClient)
    // TON balance via TonAPI
    tbs := tonbalance.NewService(cfg.TonAPIBaseURL, cfg.TonAPIToken)
	gh := NewGiveawayHandlersFiber(gs, chs, tgClient, us, tbs)

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
