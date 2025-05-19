package main

import (
	"giveaway-tool-backend/internal/config"
	userhttp "giveaway-tool-backend/internal/features/user/delivery/http"
	userredis "giveaway-tool-backend/internal/features/user/repository/redis"
	"giveaway-tool-backend/internal/features/user/service"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.MustLoad()
	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	userRepo := userredis.NewUserRepository(rdb)
	userSvc := service.NewUserService(userRepo)
	userHandler := userhttp.NewUserHandler(userSvc)

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	v1 := router.Group("/api/v1")
	userHandler.RegisterRoutes(v1)

	addr := ":" + cfg.Port
	log.Printf("⇨ listen %s …", addr)
	log.Fatal(router.Run(addr))
}
