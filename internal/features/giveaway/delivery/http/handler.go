package http

import (
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/models/dto"
	"giveaway-tool-backend/internal/features/giveaway/service"
	"giveaway-tool-backend/internal/platform/telegram"
	"net/http"
	"os"
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
	service        service.GiveawayService
	telegramClient *telegram.Client
}

func NewGiveawayHandler(service service.GiveawayService) *GiveawayHandler {
	return &GiveawayHandler{
		service:        service,
		telegramClient: telegram.NewClient(),
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

	// Add route for checking bot existence in a channel
	router.POST("/bot/check", h.checkBotInChannel)
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
		modelInput.Requirements = input.Requirements[0].Requirements
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
		case service.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case service.ErrNotOwner:
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
		case service.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case service.ErrNotOwner:
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
		case service.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case service.ErrNotOwner:
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
		case service.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "giveaway not found"})
		case service.ErrNotOwner:
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

	// Получаем информацию о канале и его числовой ID
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

	// Проверяем права бота используя числовой ID
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

	// Возвращаем полную информацию о канале и статусе бота
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
