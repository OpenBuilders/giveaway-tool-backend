package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/open-builders/giveaway-backend/internal/config"
	apphttp "github.com/open-builders/giveaway-backend/internal/http"
	"github.com/open-builders/giveaway-backend/internal/platform/db"
	redisplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
	pgrepo "github.com/open-builders/giveaway-backend/internal/repository/postgres"
	gsvc "github.com/open-builders/giveaway-backend/internal/service/giveaway"
)

func main() {
	// Create cancellable root context for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Load local environment variables from .env files for non-Docker/dev runs.
	_ = godotenv.Load()                 // loads ".env" if present (does not override existing env)
	_ = godotenv.Overload(".env.local") // optional: allow .env.local to override

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load: %v", err)
	}

	pg, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres open: %v", err)
	}
	defer pg.Close()

	rdb, err := redisplatform.Open(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("redis open: %v", err)
	}
	defer rdb.Close()

	app := apphttp.NewFiberApp(pg, rdb, cfg)

	// Start background worker for finishing expired giveaways
	expSvc := gsvc.NewService(pgrepo.NewGiveawayRepository(pg))
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.GiveawayExpireIntervalSec) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := expSvc.FinishExpired(context.Background()); err != nil {
					log.Printf("finish expired error: %v", err)
				} else if n > 0 {
					log.Printf("finished %d expired giveaways", n)
				}
			}
		}
	}()
	go func() {
		log.Printf("HTTP server (Fiber) listening on %s", cfg.HTTPAddr)
		if err := app.Listen(cfg.HTTPAddr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	stop()
	cancel := func() {} // no-op for consistent pattern with net/http version
	defer cancel()
	// Fiber graceful shutdown
	if err := app.Shutdown(); err != nil {
		log.Printf("server shutdown: %v", err)
	}
	log.Println("server stopped")
}
