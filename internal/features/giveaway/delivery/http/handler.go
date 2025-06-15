package http

import (
	"fmt"
	channelservice "giveaway-tool-backend/internal/features/channel/service"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/models/dto"
	giveawayservice "giveaway-tool-backend/internal/features/giveaway/service"
	"giveaway-tool-backend/internal/platform/telegram"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"
)

// containsString проверяет наличие строки в срезе
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

type GiveawayHandler struct {
	service        giveawayservice.GiveawayService
	telegramClient *telegram.Client
	channelService channelservice.ChannelService
}

func NewGiveawayHandler(service giveawayservice.GiveawayService, channelService channelservice.ChannelService) *GiveawayHandler {
	return &GiveawayHandler{
		service:        service,
		telegramClient: telegram.NewClient(),
		channelService: channelService,
	}
}

func (h *GiveawayHandler) RegisterRoutes(router *gin.RouterGroup) {
	giveaways := router.Group("/giveaways")
	{
		giveaways.POST("", h.create)
		giveaways.GET("/:id", h.getByID)
		giveaways.PUT("/:id", h.update)
		giveaways.DELETE("/:id", h.delete)
		giveaways.POST("/:id/join", h.join)
		giveaways.GET("/:id/participants", h.getParticipants)
		giveaways.GET("/:id/winners", h.getWinners)
		giveaways.POST("/:id/tickets", h.grantTickets)
		giveaways.POST("/:id/history", h.moveToHistory)
		giveaways.GET("/top", h.getTopGiveaways)
		giveaways.GET("/me/active", h.getMyActiveGiveaways)
		giveaways.GET("/me/history", h.getMyGiveawaysHistory)
		giveaways.GET("/me/participated", h.getParticipatedGiveaways)
		giveaways.GET("/me/participation/history", h.getParticipationHistory)
		giveaways.GET("/me/all", h.getAllMyGiveaways)
		giveaways.POST("/:id/cancel", h.cancelGiveaway)
		giveaways.POST("/:id/recreate", h.recreateGiveaway)
		giveaways.POST("/parse-ids", h.parseIDsFile)
		giveaways.GET("/:id/check-requirements", h.checkRequirements)
	}

	prizes := router.Group("/prizes")
	{
		prizes.GET("/templates", h.getPrizeTemplates)
		prizes.POST("/custom", h.createCustomPrize)
	}

	requirements := router.Group("/requirements")
	{
		requirements.GET("/templates", h.getRequirementsTemplates)
	}

	// channels := router.Group("/channels")
	// {
	// 	channels.GET("/me", h.getUserChannels)
	// 	channels.GET(":username/info", h.getPublicChannelInfo)
	// }

	// Add route for checking bot existence in a channel
	router.POST("/bot/check", h.checkBotInChannel)
	router.POST("/bot/check-bulk", h.checkBotInChannelsBulk)
}

// @title Giveaway API
// @version 1.0
// @description API for managing giveaways in Telegram Mini App

// @securityDefinitions.apikey TelegramInitData
// @in header
// @name init_data
// @description Telegram Mini App init_data for authentication

// @Summary Создать новый розыгрыш
// @Description Создает новый розыгрыш с указанными параметрами, включая требования для участия
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param input body dto.GiveawayCreateRequest true "Данные для создания розыгрыша"
// @Success 200 {object} models.GiveawayResponse "Созданный розыгрыш"
// @Failure 400 {object} models.ErrorResponse "Ошибка валидации"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways [post]
func (h *GiveawayHandler) create(c *gin.Context) {
	var input dto.GiveawayCreateRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, prize := range input.Prizes {
		// Проверяем place
		if str, ok := prize.Place.(string); ok {
			if str != "all" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "if place is string, it must be 'all'"})
				return
			}
		} else if num, ok := prize.Place.(float64); ok {
			if num < 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "prize place must be greater than 0"})
				return
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "place must be either number > 0 or string 'all'"})
			return
		}

		validPrizeTypes := []models.PrizeType{models.PrizeTypeTelegramStars, models.PrizeTypeTelegramPremium, models.PrizeTypeTelegramGifts, models.PrizeTypeTelegramStickers, models.PrizeTypeCustom}
		validType := false
		for _, t := range validPrizeTypes {
			if prize.PrizeType == t {
				validType = true
				break
			}
		}
		if !validType {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid prize_type: %s. Must be one of the valid prize types", prize.PrizeType)})
			return
		}

		if prize.PrizeType == models.PrizeTypeCustom {
			if prize.Fields == nil || len(prize.Fields) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "custom prize must have at least one field"})
				return
			}

			for _, field := range prize.Fields {
				validFieldTypes := []string{"text", "number"}
				if !containsString(validFieldTypes, field.Type) {
					c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid field type: %s. Must be one of: %v", field.Type, validFieldTypes)})
					return
				}
			}
		}
	}

	// Валидация требований
	// if input.Requirements != nil && input.Requirements.Enabled {
	// 	if input.Requirements.JoinType != "all" && input.Requirements.JoinType != "any" {
	// 		c.JSON(http.StatusBadRequest, gin.H{"error": "join_type must be either 'all' or 'any'"})
	// 		return
	// 	}
	// }

	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	// Convert DTO to model type
	modelInput := models.GiveawayCreate{
		Title:           input.Title,
		Description:     input.Description,
		Duration:        input.Duration,
		MaxParticipants: input.MaxParticipants,
		WinnersCount:    input.WinnersCount,
		Prizes:          input.Prizes,
		AutoDistribute:  input.AutoDistribute,
		AllowTickets:    input.AllowTickets,
	}

	// Handle requirements if present
	if len(input.Requirements) > 0 {
		modelInput.Requirements = input.Requirements
	}

	giveaway, err := h.service.Create(c.Request.Context(), userData.ID, &modelInput)
	if err != nil {
		if os.Getenv("DEBUG") == "true" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "debug_info": fmt.Sprintf("%+v", err)})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, giveaway)
}

// @Summary Get giveaway by ID
// @Description Get detailed information about a giveaway
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "Giveaway ID"
// @Success 200 {object} models.GiveawayResponse
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 404 {object} models.ErrorResponse "Giveaway not found"
// @Router /giveaways/{id} [get]
func (h *GiveawayHandler) getByID(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaway, err := h.service.GetByIDWithUser(c.Request.Context(), c.Param("id"), userData.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		return
	}

	c.JSON(http.StatusOK, giveaway)
}

// @Summary Get my giveaways
// @Description Get list of active and completed giveaways created by the current user
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.GiveawayResponse
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Router /giveaways [get]
func (h *GiveawayHandler) getMyGiveaways(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaways, err := h.service.GetByCreator(c.Request.Context(), userData.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// @Summary Update giveaway
// @Description Update giveaway details (only within first 5 minutes and only by creator)
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "Giveaway ID"
// @Param giveaway body models.GiveawayUpdate true "Giveaway update data"
// @Success 200 {object} models.GiveawayResponse
// @Failure 400 {object} models.ErrorResponse "Invalid input or giveaway not editable"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 403 {object} models.ErrorResponse "Not the creator"
// @Failure 404 {object} models.ErrorResponse "Giveaway not found"
// @Router /giveaways/{id} [put]
func (h *GiveawayHandler) update(c *gin.Context) {
	var input models.GiveawayUpdate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaway, err := h.service.Update(c.Request.Context(), userData.ID, c.Param("id"), &input)
	if err != nil {
		switch err {
		case giveawayservice.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case giveawayservice.ErrNotOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not the owner of this giveaway"})
		case models.ErrGiveawayNotEditable:
			c.JSON(http.StatusBadRequest, gin.H{"error": "giveaway can only be edited within first 5 minutes"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, giveaway)
}

// @Summary Delete giveaway
// @Description Delete a giveaway (only by creator)
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "Giveaway ID"
// @Success 204 "No Content"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /giveaways/{id} [delete]
func (h *GiveawayHandler) delete(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	err := h.service.Delete(c.Request.Context(), userData.ID, c.Param("id"))
	if err != nil {
		switch err {
		case giveawayservice.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case giveawayservice.ErrNotOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not the owner of this giveaway"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Присоединиться к розыгрышу
// @Description Добавляет пользователя как участника розыгрыша. Если у розыгрыша есть требования (подписка на канал или буст), они будут проверены перед добавлением участника. В случае ошибки RPS от Telegram API, проверки будут пропущены.
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "ID розыгрыша"
// @Success 200 {object} models.SuccessResponse "Успешное присоединение к розыгрышу"
// @Failure 400 {object} models.ErrorResponse "Неверный ID розыгрыша"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 403 {object} models.ErrorResponse "Нельзя присоединиться (не выполнены требования или розыгрыш завершен)"
// @Failure 404 {object} models.ErrorResponse "Розыгрыш не найден"
// @Failure 429 {object} models.ErrorResponse "Превышен лимит участников"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/{id}/join [post]
func (h *GiveawayHandler) join(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	err := h.service.Join(c.Request.Context(), userData.ID, c.Param("id"))
	if err != nil {
		switch err {
		case models.ErrGiveawayEnded:
			c.JSON(http.StatusBadRequest, gin.H{"error": "giveaway has ended"})
		case models.ErrMaxParticipantsReached:
			c.JSON(http.StatusBadRequest, gin.H{"error": "maximum number of participants reached"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.Status(http.StatusOK)
}

// @Summary Get participants
// @Description Get list of participants for a giveaway
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "Giveaway ID"
// @Success 200 {array} string
// @Failure 404 {object} models.ErrorResponse
// @Router /giveaways/{id}/participants [get]
func (h *GiveawayHandler) getParticipants(c *gin.Context) {
	participants, err := h.service.GetParticipants(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		return
	}

	c.JSON(http.StatusOK, participants)
}

// @Summary Get prize templates
// @Description Get list of available prize templates
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.PrizeTemplate
// @Router /giveaways/prizes/templates [get]
func (h *GiveawayHandler) getPrizeTemplates(c *gin.Context) {
	templates, err := h.service.GetPrizeTemplates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, templates)
}

// @Summary Get requirement templates
// @Description Get list of available requirement templates for giveaways
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.RequirementTemplate
// @Router /requirements/templates [get]
func (h *GiveawayHandler) getRequirementsTemplates(c *gin.Context) {
	templates, err := h.service.GetRequirementTemplates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, templates)
}

// @Summary Create custom prize
// @Description Create a new custom prize
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param prize body models.CustomPrizeCreate true "Custom prize data"
// @Success 201 {object} models.Prize
// @Failure 400 {object} models.ErrorResponse
// @Router /giveaways/prizes/custom [post]
func (h *GiveawayHandler) createCustomPrize(c *gin.Context) {
	var input models.CustomPrizeCreate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	prize, err := h.service.CreateCustomPrize(c.Request.Context(), &input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, prize)
}

// @Summary Get winners
// @Description Get list of winners for a completed giveaway (only for creator)
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "Giveaway ID"
// @Success 200 {array} models.Winner
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 403 {object} models.ErrorResponse "Not the creator"
// @Failure 404 {object} models.ErrorResponse "Giveaway not found"
// @Router /giveaways/{id}/winners [get]
func (h *GiveawayHandler) getWinners(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	winners, err := h.service.GetWinners(c.Request.Context(), userData.ID, c.Param("id"))
	if err != nil {
		switch err {
		case giveawayservice.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case giveawayservice.ErrNotOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": "only creator can view winners"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, winners)
}

// @Summary Добавить билеты участнику
// @Description Добавляет указанное количество билетов участнику розыгрыша. Билеты увеличивают шанс на победу.
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "ID розыгрыша"
// @Param input body models.TicketGrant true "Количество билетов"
// @Success 200 {object} models.SuccessResponse "Билеты успешно добавлены"
// @Failure 400 {object} models.ErrorResponse "Неверные данные"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 403 {object} models.ErrorResponse "Билеты не разрешены или пользователь не является участником"
// @Failure 404 {object} models.ErrorResponse "Розыгрыш не найден"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/{id}/tickets [post]
func (h *GiveawayHandler) grantTickets(c *gin.Context) {
	// Проверяем аутентификацию
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	// Проверяем права администратора
	adminIDs := strings.Split(os.Getenv("ADMIN_IDS"), ",")
	isAdmin := false
	for _, adminID := range adminIDs {
		if adminID == strconv.FormatInt(userData.ID, 10) {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "only admins can grant tickets"})
		return
	}

	var input models.TicketGrant
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Проверяем существование розыгрыша
	giveaway, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		return
	}

	// Проверяем, разрешены ли билеты для этого розыгрыша
	if !giveaway.AllowTickets {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tickets are not allowed for this giveaway"})
		return
	}

	// Добавляем билеты
	if err := h.service.AddTickets(c.Request.Context(), input.UserID, c.Param("id"), input.TicketCount); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// @Summary Получить исторические розыгрыши
// @Description Возвращает список завершенных розыгрышей пользователя
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.SwaggerGiveawayResponse "Список исторических розыгрышей"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/historical [get]
func (h *GiveawayHandler) getHistoricalGiveaways(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaways, err := h.service.GetHistoricalGiveaways(c.Request.Context(), userData.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// @Summary Переместить розыгрыш в историю
// @Description Перемещает завершенный розыгрыш в историю
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "ID розыгрыша"
// @Success 200 {object} models.SuccessResponse "Розыгрыш перемещен в историю"
// @Failure 400 {object} models.ErrorResponse "Розыгрыш не завершен"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 403 {object} models.ErrorResponse "Нет прав на перемещение"
// @Failure 404 {object} models.ErrorResponse "Розыгрыш не найден"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/{id}/history [post]
func (h *GiveawayHandler) moveToHistory(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	err := h.service.MoveToHistory(c.Request.Context(), userData.ID, c.Param("id"))
	if err != nil {
		switch err {
		case giveawayservice.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case giveawayservice.ErrNotOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not the owner of this giveaway"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.Status(http.StatusOK)
}

// @Summary Получить мои активные розыгрыши
// @Description Возвращает список активных розыгрышей, созданных пользователем
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.SwaggerGiveawayDetailedResponse "Список активных розыгрышей"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/my/active [get]
func (h *GiveawayHandler) getMyActiveGiveaways(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaways, err := h.service.GetCreatedGiveaways(c.Request.Context(), userData.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// @Summary Получить историю моих розыгрышей
// @Description Возвращает список завершенных розыгрышей, созданных пользователем
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.SwaggerGiveawayDetailedResponse "Список завершенных розыгрышей"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/my/history [get]
func (h *GiveawayHandler) getMyGiveawaysHistory(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaways, err := h.service.GetCreatedGiveawaysHistory(c.Request.Context(), userData.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// @Summary Получить розыгрыши, в которых я участвую
// @Description Возвращает список активных розыгрышей, в которых пользователь принимает участие
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.SwaggerGiveawayDetailedResponse "Список розыгрышей"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/participated [get]
func (h *GiveawayHandler) getParticipatedGiveaways(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaways, err := h.service.GetParticipatedGiveaways(c.Request.Context(), userData.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// @Summary Получить историю участия в розыгрышах
// @Description Возвращает список завершенных розыгрышей, в которых пользователь принимал участие
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.SwaggerGiveawayDetailedResponse "Список розыгрышей"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/participated/history [get]
func (h *GiveawayHandler) getParticipationHistory(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaways, err := h.service.GetParticipationHistory(c.Request.Context(), userData.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// @Summary Получить все мои розыгрыши
// @Description Возвращает список всех розыгрышей, созданных пользователем (активные и исторические)
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Success 200 {array} models.SwaggerGiveawayDetailedResponse "Список всех розыгрышей"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/me/all [get]
func (h *GiveawayHandler) getAllMyGiveaways(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaways, err := h.service.GetAllCreatedGiveaways(c.Request.Context(), userData.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// @Summary Отменить розыгрыш
// @Description Отменяет активный розыгрыш. Возможно только если розыгрыш не старше 5 минут и имеет не более 10 участников.
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "ID розыгрыша"
// @Success 200 {object} models.SuccessResponse "Розыгрыш успешно отменен"
// @Failure 400 {object} models.ErrorResponse "Невозможно отменить розыгрыш"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 403 {object} models.ErrorResponse "Нет прав на отмену"
// @Failure 404 {object} models.ErrorResponse "Розыгрыш не найден"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/{id}/cancel [post]
func (h *GiveawayHandler) cancelGiveaway(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	err := h.service.CancelGiveaway(c.Request.Context(), userData.ID, c.Param("id"))
	if err != nil {
		switch err {
		case giveawayservice.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case giveawayservice.ErrNotOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not the owner of this giveaway"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.Status(http.StatusOK)
}

// @Summary Пересоздать розыгрыш
// @Description Создает новый розыгрыш на основе существующего отмененного розыгрыша
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "ID розыгрыша"
// @Success 200 {object} models.GiveawayResponse "Новый розыгрыш"
// @Failure 400 {object} models.ErrorResponse "Невозможно пересоздать розыгрыш"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 403 {object} models.ErrorResponse "Нет прав на пересоздание"
// @Failure 404 {object} models.ErrorResponse "Розыгрыш не найден"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/{id}/recreate [post]
func (h *GiveawayHandler) recreateGiveaway(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	giveaway, err := h.service.RecreateGiveaway(c.Request.Context(), userData.ID, c.Param("id"))
	if err != nil {
		switch err {
		case giveawayservice.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case giveawayservice.ErrNotOwner:
			c.JSON(http.StatusForbidden, gin.H{"error": "you are not the owner of this giveaway"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, giveaway)
}

// @Summary Парсинг файла с ID
// @Description Принимает txt файл с ID (через запятую или новую строку) и возвращает обработанный список
// @Tags giveaways
// @Accept multipart/form-data
// @Produce json
// @Security TelegramInitData
// @Param file formData file true "Текстовый файл с ID"
// @Success 200 {object} models.ParseIDsResponse
// @Failure 400 {object} models.ErrorResponse "Ошибка в формате файла"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/parse-ids [post]
func (h *GiveawayHandler) parseIDsFile(c *gin.Context) {
	// Проверяем аутентификацию
	_, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Получаем файл из запроса
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	// Проверяем расширение файла
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".txt") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only .txt files are allowed"})
		return
	}

	// Открываем файл
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
		return
	}
	defer src.Close()

	// Читаем содержимое файла
	content, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	// Обрабатываем содержимое
	text := string(content)

	// Сначала разбиваем по переносу строки
	lines := strings.Split(text, "\n")

	// Создаем множество для уникальных ID
	uniqueIDs := make(map[string]bool)

	// Обрабатываем каждую строку
	for _, line := range lines {
		// Убираем пробелы в начале и конце
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Разбиваем строку по запятым
		ids := strings.Split(line, ",")
		for _, id := range ids {
			// Убираем пробелы для каждого ID
			id = strings.TrimSpace(id)
			if id != "" {
				uniqueIDs[id] = true
			}
		}
	}

	// Преобразуем множество в слайс
	result := make([]string, 0, len(uniqueIDs))
	for id := range uniqueIDs {
		result = append(result, id)
	}

	// Сортируем результат для стабильного вывода
	sort.Strings(result)

	response := models.ParseIDsResponse{
		TotalIDs: len(result),
		IDs:      result,
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Проверить требования для участия в розыгрыше
// @Description Проверяет выполнение всех требований для участия в розыгрыше (подписка на каналы, бусты и т.д.)
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param id path string true "ID розыгрыша"
// @Success 200 {object} models.RequirementsCheckResponse "Результаты проверки требований"
// @Failure 400 {object} models.ErrorResponse "Неверный ID розыгрыша"
// @Failure 401 {object} models.ErrorResponse "Не авторизован"
// @Failure 404 {object} models.ErrorResponse "Розыгрыш не найден"
// @Failure 500 {object} models.ErrorResponse "Внутренняя ошибка сервера"
// @Router /giveaways/{id}/check-requirements [get]
func (h *GiveawayHandler) checkRequirements(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userData := user.(initdata.User)

	results, err := h.service.CheckRequirements(c.Request.Context(), userData.ID, c.Param("id"))
	if err != nil {
		switch err {
		case giveawayservice.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, results)
}

type ChannelInfo struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	AvatarURL  string `json:"avatar_url"`
	ChannelURL string `json:"channel_url"`
}

// @Summary Get user channels
// @Description Get list of channels where user is admin with their titles and avatars
// @Tags channels
// @Accept json
// @Produce json
// @Success 200 {object} map[string][]ChannelInfo
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /channels/me [get]
func (h *GiveawayHandler) getUserChannels(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	channels, err := h.channelService.GetUserChannels(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user channels"})
		return
	}

	response := make([]ChannelInfo, 0, len(channels))
	for _, channelID := range channels {
		title, err := h.channelService.GetChannelTitle(c.Request.Context(), channelID)
		if err != nil {
			continue
		}

		username, err := h.channelService.GetChannelUsername(c.Request.Context(), channelID)
		if err != nil {
			continue
		}

		avatarURL, err := h.channelService.GetChannelAvatar(c.Request.Context(), channelID)
		if err != nil {
			continue
		}

		response = append(response, ChannelInfo{
			ID:         channelID,
			Title:      title,
			AvatarURL:  avatarURL,
			ChannelURL: "https://t.me/" + username,
		})
	}

	c.JSON(http.StatusOK, gin.H{"channels": response})
}

// @Summary Check if a bot exists in a channel
// @Description Check if the Telegram bot exists in a channel and has access to member information
// @Tags bot
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param request body map[string]string true "Request with channel username"
// @Success 200 {object} map[string]interface{} "Channel and bot status information"
// @Failure 400 {object} models.ErrorResponse "Bad request"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 404 {object} models.ErrorResponse "Bot not found in channel"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /bot/check [post]
func (h *GiveawayHandler) checkBotInChannel(c *gin.Context) {
	// Check authentication
	_, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse request body
	var requestBody struct {
		Username string `json:"username"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	if requestBody.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username is required"})
		return
	}

	// Ensure username is properly formatted with @ prefix if needed
	username := requestBody.Username
	if !strings.HasPrefix(username, "@") {
		username = "@" + username
	}

	chat, err := h.telegramClient.GetChat(username)
	if err != nil {
		if os.Getenv("DEBUG") == "true" {
			c.JSON(http.StatusNotFound, gin.H{
				"error":      "Channel not found or is not accessible",
				"debug_info": err.Error(),
			})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Channel not found or is not accessible"})
		}
		return
	}

	chatMember, err := h.telegramClient.GetBotChatMember(fmt.Sprintf("%d", chat.ID))
	if err != nil {
		if os.Getenv("DEBUG") == "true" {
			c.JSON(http.StatusNotFound, gin.H{
				"error":      "Bot is not a member of this channel",
				"debug_info": err.Error(),
			})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bot is not a member of this channel"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"channel": gin.H{
			"id":       chat.ID,
			"type":     chat.Type,
			"title":    chat.Title,
			"username": chat.Username,
		},
		"bot_status": gin.H{
			"status":            chatMember.Status,
			"can_check_members": chatMember.CanInviteUsers,
		},
	})
}

// @Summary Get top giveaways
// @Description Get top giveaways sorted by participants count
// @Tags giveaways
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param limit query int false "Number of giveaways to return (default 10, max 50)"
// @Success 200 {array} models.GiveawayResponse "Top giveaways"
// @Failure 400 {object} models.ErrorResponse "Invalid request"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /giveaways/top [get]
func (h *GiveawayHandler) getTopGiveaways(c *gin.Context) {
	// Parse limit parameter
	limit := 10 // default limit
	if limitStr := c.Query("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter"})
			return
		}
		if parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	// Cap maximum limit
	if limit > 50 {
		limit = 50
	}

	// Get top giveaways
	giveaways, err := h.service.GetTopGiveaways(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, giveaways)
}

// ErrorResponse представляет структуру ответа с ошибкой
type ErrorResponse struct {
	Error string `json:"error"`
}

// @Summary Get public channel info
// @Description Get public information about a channel including its URL and avatar
// @Tags channels
// @Accept json
// @Produce json
// @Param username path string true "Channel username without @"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /channels/{username}/info [get]
func (h *GiveawayHandler) getPublicChannelInfo(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "username is required"})
		return
	}

	info, err := h.channelService.GetPublicChannelInfo(c.Request.Context(), username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// BulkBotCheckRequest описывает входные данные для bulk-проверки
type BulkBotCheckRequest struct {
	Usernames []string `json:"usernames"`
}

// BulkBotCheckResult описывает результат для одного канала
type BulkBotCheckResult struct {
	Username  string      `json:"username"`
	Ok        bool        `json:"ok"`
	Channel   interface{} `json:"channel,omitempty"`
	BotStatus interface{} `json:"bot_status,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// @Summary Bulk check if a bot exists in channels
// @Description Check if the Telegram bot exists in multiple channels and has access to member information
// @Tags bot
// @Accept json
// @Produce json
// @Security TelegramInitData
// @Param request body BulkBotCheckRequest true "Request with channel usernames"
// @Success 200 {object} map[string][]BulkBotCheckResult
// @Failure 400 {object} models.ErrorResponse "Bad request"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /bot/check-bulk [post]
func (h *GiveawayHandler) checkBotInChannelsBulk(c *gin.Context) {
	_, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req BulkBotCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Usernames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format or empty usernames"})
		return
	}

	results := make([]BulkBotCheckResult, 0, len(req.Usernames))
	for _, username := range req.Usernames {
		uname := username
		if uname == "" {
			results = append(results, BulkBotCheckResult{Username: username, Ok: false, Error: "Empty username"})
			continue
		}
		if uname[0] != '@' {
			uname = "@" + uname
		}
		chat, err := h.telegramClient.GetChat(uname)
		if err != nil {
			results = append(results, BulkBotCheckResult{Username: username, Ok: false, Error: fmt.Sprintf("Channel '%s' not found or is not accessible", username)})
			continue
		}
		chatMember, err := h.telegramClient.GetBotChatMember(fmt.Sprintf("%d", chat.ID))
		if err != nil {
			results = append(results, BulkBotCheckResult{Username: username, Ok: false, Error: fmt.Sprintf("Bot is not a member of channel '%s'", username)})
			continue
		}
		results = append(results, BulkBotCheckResult{
			Username: username,
			Ok:       true,
			Channel: gin.H{
				"id":       chat.ID,
				"type":     chat.Type,
				"title":    chat.Title,
				"username": chat.Username,
			},
			BotStatus: gin.H{
				"status":            chatMember.Status,
				"can_check_members": chatMember.CanInviteUsers,
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}
