package main

import (
	"context"
	"fmt"
	"giveaway-tool-backend/internal/common/config"
	"giveaway-tool-backend/internal/common/logger"
	"giveaway-tool-backend/internal/common/middleware"
	channelhttp "giveaway-tool-backend/internal/features/channel/delivery/http"
	channelredis "giveaway-tool-backend/internal/features/channel/repository/redis"
	channelservice "giveaway-tool-backend/internal/features/channel/service"
	giveawayhttp "giveaway-tool-backend/internal/features/giveaway/delivery/http"
	giveawayredis "giveaway-tool-backend/internal/features/giveaway/repository/redis"
	giveawayservice "giveaway-tool-backend/internal/features/giveaway/service"
	redisplatform "giveaway-tool-backend/internal/platform/redis"

	// tonproofhttp "giveaway-tool-backend/internal/features/tonproof/handler/http"
	// tonproofredis "giveaway-tool-backend/internal/features/tonproof/repository/redis"
	// tonproofservice "giveaway-tool-backend/internal/features/tonproof/service"
	userhttp "giveaway-tool-backend/internal/features/user/delivery/http"
	userredis "giveaway-tool-backend/internal/features/user/repository/redis"
	userservice "giveaway-tool-backend/internal/features/user/service"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "giveaway-tool-backend/docs"
	"giveaway-tool-backend/internal/platform/telegram"

	"github.com/gin-gonic/gin"
)

// @title           Giveaway Tool API
// @version         1.0
// @description     API server for Telegram Mini App giveaways. All endpoints require init_data authentication.

// @contact.name   API Support
// @contact.url    https://t.me/seinarukiro
// @contact.email  seinarukiro@gmail.com

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey TelegramInitData
// @in header
// @name init_data
// @description Telegram Mini App init_data string for authentication

// @tag.name users
// @tag.description User management

// @tag.name giveaways
// @tag.description Giveaway management - creation, participation, viewing and administration

// @tag.name prizes
// @tag.description Prize management - templates and custom prizes

// @tag.name tickets
// @tag.description Ticket management - adding and viewing participant tickets

// @tag.name requirements
// @tag.description Participation requirements - channel subscription and boost settings

func main() {
	// Initialize configuration
	cfg := config.Load()

	// Initialize logger
	logger.Init("giveaway-tool", cfg.Debug)

	// Initialize Redis client using factory
	redisClient, err := redisplatform.CreateRedisClient(cfg)
	if err != nil {
		logger.Fatal().
			Err(err).
			Msg("Failed to create Redis client")
	}

	// Create adapter for compatibility with existing repositories
	rdb := redisplatform.NewRedisAdapter(redisClient)

	// Enable TTL notifications
	if err := rdb.ConfigSet(context.Background(), "notify-keyspace-events", "Ex").Err(); err != nil {
		logger.Fatal().
			Err(err).
			Msg("Failed to configure Redis notifications")
	}

	// Log Redis configuration
	stats := redisplatform.GetShardStats(redisClient)
	logger.Info().
		Interface("redis_stats", stats).
		Msg("Redis client initialized")

	// Initialize repositories
	userRepository := userredis.NewUserRepository(rdb.GetUnderlyingClient())
	giveawayRepository := giveawayredis.NewRedisGiveawayRepository(rdb.GetUnderlyingClient())
	channelRepository := channelredis.NewRedisChannelRepository(rdb.GetUnderlyingClient())
	// tonProofRepository := tonproofredis.NewRepository(rdb)

	// Initialize services
	userSvc := userservice.NewUserService(userRepository)
	channelSvc := channelservice.NewChannelService(channelRepository, rdb.GetUnderlyingClient(), cfg.Debug)
	giveawaySvc := giveawayservice.NewGiveawayService(giveawayRepository, rdb.GetUnderlyingClient(), cfg.Debug, channelSvc)
	// tonProofSvc := tonproofservice.NewService(tonProofRepository)
	telegramClient := telegram.NewClient()
	completionService := giveawayservice.NewCompletionService(giveawayRepository, telegramClient)
	expirationService := giveawayservice.NewExpirationService(giveawayRepository)
	queueService := giveawayservice.NewQueueService(context.Background(), giveawayRepository, expirationService)

	// Start background services
	completionService.Start()
	queueService.Start()
	defer func() {
		completionService.Stop()
		queueService.Stop()
	}()

	// Initialize HTTP handlers
	userHandler := userhttp.NewUserHandler(userSvc)
	giveawayHandler := giveawayhttp.NewGiveawayHandler(giveawaySvc, channelSvc)
	channelHandler := channelhttp.NewChannelHandler(channelSvc)
	// tonProofHandler := tonproofhttp.NewHandler(tonProofSvc)

	// Initialize Gin router
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(
		cors.New(cors.Config{
			AllowOrigins:     []string{cfg.Server.Origin},
			AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Content-Type", "Authorization", "Accept", "init_data"},
			ExposeHeaders:    []string{"Content-Length"},
			AllowCredentials: true,
		}),
		middleware.Logger(), gin.Recovery())

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API routes
	v1 := router.Group("/api/v1")
	v1.Use(
		middleware.TelegramInitDataMiddleware(),
		middleware.AutoCreateUser(userSvc),
		middleware.CheckBanned(userSvc),
	)
	{
		userHandler.RegisterRoutes(v1)
		giveawayHandler.RegisterRoutes(v1)
		channelHandler.RegisterRoutes(v1)
		// tonProofHandler.RegisterRoutes(v1)
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start server in a goroutine
	go func() {
		logger.Info().
			Int("port", cfg.Server.Port).
			Msg("Starting server...")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().
				Err(err).
				Msg("Failed to start server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("Shutting down server...")

	// Give outstanding requests 5 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().
			Err(err).
			Msg("Server forced to shutdown")
	}

	logger.Info().Msg("Server exited")
}
