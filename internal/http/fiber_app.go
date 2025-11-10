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
	notify "github.com/open-builders/giveaway-backend/internal/service/notifications"
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
	// TON Proof service (local verification). Handlers require Telegram init-data auth.
	tps := tonproof.NewService(rdb, cfg.TonProofDomain, cfg.TonProofPayloadTTLSec)
	tph := NewTonProofHandlers(tps, cfg.TonProofDomain, us)

	// Giveaway domain deps
	gRepo := pgrepo.NewGiveawayRepository(pg)
	tgClient := telegram.NewClientFromEnv()
	// Prime bot info in Redis on startup (best-effort)
	{
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = tgClient.SetBotMe(ctx, rdb)
		cancel()
	}
	notifier := notify.NewService(tgClient, chs, cfg.WebAppBaseURL, rdb, us)
	gs := gsvc.NewService(gRepo, chs).WithTelegram(tgClient).WithNotifier(notifier).WithRedis(rdb)
	// TON balance via TonAPI
	tbs := tonbalance.NewService(cfg.TonAPIBaseURL, cfg.TonAPIToken).WithCache(rdb, 0)
	gh := NewGiveawayHandlersFiber(gs, chs, tgClient, us, tbs, rdb)

	// API groups
	ttl := time.Duration(cfg.InitDataTTL) * time.Second
	api := app.Group("/api")
	v1 := api.Group("/v1", mw.InitDataMiddleware(cfg.TelegramBotToken, ttl))

	// Protected endpoints (require InitData middleware)
	uh.RegisterFiber(v1)
	gh.RegisterFiber(v1)
	tph.RegisterFiber(v1)

	// Channel handlers - split between protected and public
	avatarCache := rcache.NewChannelAvatarCache(rdb, 24*time.Hour)
	// Short-lived cache for getChat photo identifiers to reduce Telegram calls
	photoCache := rcache.NewChannelPhotoCache(rdb, 10*time.Minute)
	ch := NewChannelHandlers(tgClient, avatarCache, photoCache)
	ch.RegisterFiber(v1) // Protected: info, membership, boost

	rq := NewRequirementsHandlers(tgClient, us, tbs, chs)
	rq.RegisterFiber(v1)

	// Public endpoints (no init-data required)
	v1public := api.Group("/public")
	ch.RegisterPublicFiber(v1public) // Public: avatar only
	gh.RegisterPublicFiber(v1public) // Public: giveaways export by token

	return app
}
