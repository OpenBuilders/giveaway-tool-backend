package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/your-org/giveaway-backend/internal/config"
	apphttp "github.com/your-org/giveaway-backend/internal/http"
	"github.com/your-org/giveaway-backend/internal/platform/db"
	redisplatform "github.com/your-org/giveaway-backend/internal/platform/redis"
)

func main() {
	// Create cancellable root context for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
