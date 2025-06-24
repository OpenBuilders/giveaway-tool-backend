package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"giveaway-tool-backend/internal/common/cache"
	"giveaway-tool-backend/internal/common/config"
	"giveaway-tool-backend/internal/common/middleware"
	channelRepo "giveaway-tool-backend/internal/features/channel/repository/postgres"
	channelService "giveaway-tool-backend/internal/features/channel/service"
	giveawayRepo "giveaway-tool-backend/internal/features/giveaway/repository/postgres"
	giveawayService "giveaway-tool-backend/internal/features/giveaway/service"
	userRepo "giveaway-tool-backend/internal/features/user/repository/postgres"
	userService "giveaway-tool-backend/internal/features/user/service"
	"giveaway-tool-backend/internal/platform/postgres"
	"giveaway-tool-backend/internal/platform/redis"
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
	// Загружаем переменные окружения
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Инициализируем конфигурацию
	cfg := config.Load()

	// Инициализируем логгер
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting Giveaway Tool Backend",
		zap.String("version", "1.0.0"),
		zap.Bool("debug", cfg.Debug),
	)

	// Инициализируем базу данных
	postgresClient, err := postgres.NewClient(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer postgresClient.Close()

	logger.Info("Database connection established")

	// Инициализируем Redis
	redisClient, err := redis.CreateRedisClient(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Инициализируем кэш
	cacheService := cache.NewCacheService(redisClient)
	logger.Info("Cache service initialized")

	// Инициализируем репозитории
	userRepository := userRepo.NewPostgresRepository(postgresClient.GetDB())
	channelRepository := channelRepo.NewPostgresRepository(postgresClient.GetDB())
	giveawayRepository := giveawayRepo.NewPostgresRepository(postgresClient.GetDB())

	logger.Info("Repositories initialized")

	// Инициализируем сервисы
	userSvc := userService.NewUserService(userRepository, logger)
	channelSvc := channelService.NewChannelService(channelRepository, cacheService, cfg.Debug, logger)
	giveawaySvc := giveawayService.NewGiveawayService(giveawayRepository, cacheService, cfg, channelSvc, log.New(os.Stdout, "[GiveawayService] ", log.LstdFlags))

	logger.Info("Services initialized")

	// Настраиваем Gin
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Добавляем middleware
	router.Use(middleware.RequestID())
	router.Use(middleware.ErrorHandler(logger))
	router.Use(gin.Recovery())

	// Настраиваем CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{cfg.Server.Origin}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Content-Type", "Authorization", "Accept", "init_data"}
	router.Use(cors.New(corsConfig))

	logger.Info("Middleware configured")

	// Настраиваем роуты
	setupRoutes(router, userSvc, channelSvc, giveawaySvc, logger, postgresClient, redisClient)

	logger.Info("Routes configured")

	// Создаем HTTP сервер
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Запускаем сервер в горутине
	go func() {
		logger.Info("Starting HTTP server", zap.Int("port", cfg.Server.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Ждем сигнала для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

func setupRoutes(router *gin.Engine, userSvc userService.UserService, channelSvc channelService.ChannelService, giveawaySvc giveawayService.GiveawayService, logger *zap.Logger, postgresClient *postgres.Client, redisClient redis.RedisClient) {
	// Группа API v1
	v1 := router.Group("/api/v1")
	{
		// Роуты пользователей
		users := v1.Group("/users")
		{
			users.GET("/:id", wrapHandler(func(c *gin.Context) {
				// TODO: Implement user get handler
				c.JSON(http.StatusOK, gin.H{"message": "User endpoint"})
			}, logger))
		}

		// Роуты каналов
		channels := v1.Group("/channels")
		{
			channels.GET("/:id", wrapHandler(func(c *gin.Context) {
				// TODO: Implement channel get handler
				c.JSON(http.StatusOK, gin.H{"message": "Channel endpoint"})
			}, logger))
		}

		// Роуты гивов
		giveaways := v1.Group("/giveaways")
		{
			giveaways.GET("/:id", wrapHandler(func(c *gin.Context) {
				// TODO: Implement giveaway get handler
				c.JSON(http.StatusOK, gin.H{"message": "Giveaway endpoint"})
			}, logger))
		}
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().UTC(),
			"service":   "giveaway-tool-backend",
		})
	})

	// Liveness probe
	router.GET("/live", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Readiness probe
	router.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		// Проверка Postgres
		if err := postgresClient.HealthCheck(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "unready",
				"error":   "postgres unavailable",
				"details": err.Error(),
			})
			return
		}

		// Проверка Redis
		if err := redisClient.Ping(ctx).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "unready",
				"error":   "redis unavailable",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"timestamp": time.Now().UTC(),
			"service":   "giveaway-tool-backend",
		})
	})
}

// wrapHandler оборачивает обработчики для автоматической обработки ошибок
func wrapHandler(handler gin.HandlerFunc, logger *zap.Logger) gin.HandlerFunc {
	return middleware.HandleErrorWrapper(logger)(handler)
}
