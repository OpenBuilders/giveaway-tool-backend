package main

import (
	"giveaway-tool-backend/internal/handler"
	"giveaway-tool-backend/internal/redis"
	"giveaway-tool-backend/internal/repository"
	"giveaway-tool-backend/internal/service"
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"giveaway-tool-backend/internal/config"
)

func main() {
	cfg := config.MustLoad()
	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
	}

	rdb := redis.New(cfg.RedisHost, cfg.RedisPassword, cfg.RedisDB)

	userRepo := repository.NewUserRepo(rdb)
	userSvc := service.NewUserService(userRepo)

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	group := router.Group("/api/" + os.Getenv("API_VERSION"))
	router.RouterGroup = *group

	handler.NewUserHandler(router, userSvc)

	addr := ":" + cfg.Port
	log.Printf("⇨ listen %s …", addr)
	log.Fatal(router.Run(addr))
}
