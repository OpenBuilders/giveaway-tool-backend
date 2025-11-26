package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	rcache "github.com/open-builders/giveaway-backend/internal/cache/redis"
	"github.com/open-builders/giveaway-backend/internal/config"
	apphttp "github.com/open-builders/giveaway-backend/internal/http"
	"github.com/open-builders/giveaway-backend/internal/platform/db"
	redisplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
	pgrepo "github.com/open-builders/giveaway-backend/internal/repository/postgres"
	"github.com/open-builders/giveaway-backend/internal/service/channels"
	gsvc "github.com/open-builders/giveaway-backend/internal/service/giveaway"
	notify "github.com/open-builders/giveaway-backend/internal/service/notifications"
	tg "github.com/open-builders/giveaway-backend/internal/service/telegram"
	usersvc "github.com/open-builders/giveaway-backend/internal/service/user"
	"github.com/open-builders/giveaway-backend/internal/workers"
	migfs "github.com/open-builders/giveaway-backend/migrations"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
)

func main() {
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

	// Auto-migrate DB on start if configured
	if cfg.DBAutoMigrate {
		if err := goose.SetDialect("postgres"); err != nil {
			log.Fatalf("goose dialect: %v", err)
		}
		goose.SetBaseFS(migfs.Files)
		if err := goose.Up(pg, "."); err != nil {
			log.Fatalf("migrations up: %v", err)
		}
	}

	rdb, err := redisplatform.Open(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("redis open: %v", err)
	}
	defer rdb.Close()

	app := apphttp.NewFiberApp(pg, rdb, cfg)

	// Start background worker for finishing expired giveaways
	chs := channels.NewService(rdb)
	expRepo := pgrepo.NewGiveawayRepository(pg)
	expSvc := gsvc.NewService(expRepo, chs)
	// Attach Telegram + notifications so worker can emit completion messages
	tgClient := tg.NewClientFromEnv()
	
	// Upload animations if needed (requires TELEGRAM_ADMIN_ID)
	if cfg.TelegramAdminID != 0 {
		uploadAnimation := func(key, filePath string) {
			_, err := rdb.Get(ctx, key).Result()
			if err == redis.Nil {
				// Key missing, upload
				if _, err := os.Stat(filePath); err == nil {
					log.Printf("Uploading animation %s to Telegram...", filePath)
					fileID, err := tgClient.UploadAnimation(ctx, cfg.TelegramAdminID, filePath)
					if err != nil {
						log.Printf("Failed to upload animation %s: %v", filePath, err)
					} else {
						if err := rdb.Set(ctx, key, fileID, 0).Err(); err != nil {
							log.Printf("Failed to save file_id for %s: %v", key, err)
						} else {
							log.Printf("Uploaded animation %s, file_id: %s", filePath, fileID)
						}
					}
				} else {
					// Only log if we expected it to be there; since we might not have both files yet
					// we can log a warning or info.
					log.Printf("Animation file not found: %s (skipping upload for %s)", filePath, key)
				}
			}
		}

		uploadAnimation("tg:file_id:animation:started", "assets/static/Giveaway.mp4")
		uploadAnimation("tg:file_id:animation:finished", "assets/static/Giveaway.mp4")
	} else {
		log.Println("TELEGRAM_ADMIN_ID not set, skipping animation uploads")
	}

	// user service for username/first name in notifications
	urepo := pgrepo.NewUserRepository(pg)
	ucache := rcache.NewUserCache(rdb, 5*time.Second)
	usvc := usersvc.NewService(urepo, ucache)
	notifier := notify.NewService(tgClient, chs, cfg.WebAppBaseURL, rdb, usvc)
	expSvc = expSvc.WithTelegram(tgClient).WithNotifier(notifier)
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

	// Start Redis stream worker
	streamWorker := workers.NewRedisStreamWorker(rdb, expRepo)
	go streamWorker.Start(ctx)

	go func() {
		log.Printf("HTTP server (Fiber) listening on %s", cfg.HTTPAddr)
		if err := app.Listen(cfg.HTTPAddr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	stop()
	cancel := func() {}
	defer cancel()

	if err := app.Shutdown(); err != nil {
		log.Printf("server shutdown: %v", err)
	}
	log.Println("server stopped")
}
