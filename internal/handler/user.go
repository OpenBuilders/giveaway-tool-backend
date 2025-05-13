package handler

import (
	"giveaway-tool-backend/internal/middleware"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"

	"giveaway-tool-backend/internal/service"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(r *gin.Engine, svc *service.UserService) {
	h := &UserHandler{svc: svc}

	g := r.Group("/users")
	g.GET("/me", middleware.InitDataMiddleware(), h.getMe)
}

type createReq struct {
	ID        string `json:"id"        binding:"required"`
	Username  string `json:"username"  binding:"required"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name"`
	IsPremium bool   `json:"is_premium" binding:"required"`
	Avatar    string `json:"avatar"    binding:"required"`
}

func (h *UserHandler) getMe(c *gin.Context) {
	user, _ := c.Get("user")
	userData, ok := user.(initdata.User)

	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user"})
		return
	}

	id := strconv.FormatInt(userData.ID, 10)

	u, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		u, err = h.svc.Create(c.Request.Context(), id, userData.Username, userData.FirstName, userData.LastName, userData.IsPremium, userData.PhotoURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, u)
}
