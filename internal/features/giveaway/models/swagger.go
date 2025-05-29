package models

// @Description Тип требования для участия в розыгрыше
type SwaggerRequirementType string

// @Description Тип чата
type SwaggerChatType string

// @Description Требование для участия в розыгрыше
type SwaggerRequirement struct {
	// @Description Название требования
	Name string `json:"name" example:"Channel Subscription"`
	// @Description Значение требования (может быть строкой или массивом строк)
	Value interface{} `json:"value" example:"@mychannel"`
	// @Description Тип требования (например, "subscription")
	Type string `json:"type" example:"subscription" enums:"subscription"`
}

// @Description Требования для участия в розыгрыше
type SwaggerRequirements []SwaggerRequirement

// @Description Данные для создания розыгрыша
type SwaggerGiveawayCreate struct {
	// @Description Название розыгрыша
	// @Required
	Title string `json:"title" binding:"required" example:"iPhone 15 Pro Giveaway"`
	// @Description Описание розыгрыша
	// @Required
	Description string `json:"description" binding:"required" example:"Разыгрываем новый iPhone 15 Pro!"`
	// @Description Длительность розыгрыша в секундах
	// @Required
	Duration int64 `json:"duration" binding:"required" example:"86400"`
	// @Description Максимальное количество участников (0 = без ограничений)
	MaxParticipants int `json:"max_participants" example:"1000"`
	// @Description Количество победителей
	// @Required
	WinnersCount int `json:"winners_count" binding:"required" example:"1"`
	// @Description Список призов по местам
	// @Required
	Prizes []PrizePlace `json:"prizes" binding:"required"`
	// @Description Автоматическая выдача призов
	AutoDistribute bool `json:"auto_distribute" example:"true"`
	// @Description Разрешены ли билеты
	AllowTickets bool `json:"allow_tickets" example:"true"`
	// @Description Требования для участия
	Requirements *Requirements `json:"requirements,omitempty"`
}

// @Description Информация о розыгрыше
type SwaggerGiveawayResponse struct {
	// @Description Уникальный идентификатор розыгрыша
	ID string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// @Description ID создателя розыгрыша
	CreatorID int64 `json:"creator_id" example:"123456789"`
	// @Description Название розыгрыша
	Title string `json:"title" example:"iPhone 15 Pro Giveaway"`
	// @Description Описание розыгрыша
	Description string `json:"description" example:"Разыгрываем новый iPhone 15 Pro!"`
	// @Description Время начала розыгрыша
	StartedAt string `json:"started_at" example:"2024-03-20T15:00:00Z"`
	// @Description Время окончания розыгрыша
	EndsAt string `json:"ends_at" example:"2024-03-21T15:00:00Z"`
	// @Description Максимальное количество участников
	MaxParticipants int `json:"max_participants,omitempty" example:"1000"`
	// @Description Количество победителей
	WinnersCount int `json:"winners_count" example:"1"`
	// @Description Статус розыгрыша
	Status string `json:"status" example:"active" enums:"active,pending,completed,history,cancelled"`
	// @Description Текущее количество участников
	ParticipantsCount int64 `json:"participants_count" example:"500"`
	// @Description Можно ли редактировать розыгрыш
	CanEdit bool `json:"can_edit" example:"true"`
	// @Description Роль текущего пользователя
	UserRole string `json:"user_role" example:"owner" enums:"owner,participant,user"`
	// @Description Список призов
	Prizes []PrizePlace `json:"prizes,omitempty"`
	// @Description Автоматическая выдача призов
	AutoDistribute bool `json:"auto_distribute" example:"true"`
	// @Description Разрешены ли билеты
	AllowTickets bool `json:"allow_tickets" example:"true"`
	// @Description Список победителей (только для завершенных розыгрышей)
	Winners []Winner `json:"winners,omitempty"`
}

// @Description Детальная информация о розыгрыше
type SwaggerGiveawayDetailedResponse struct {
	// @Description Уникальный идентификатор розыгрыша
	ID string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// @Description ID создателя розыгрыша
	CreatorID int64 `json:"creator_id" example:"123456789"`
	// @Description Имя пользователя создателя
	CreatorUsername string `json:"creator_username" example:"johndoe"`
	// @Description Название розыгрыша
	Title string `json:"title" example:"iPhone 15 Pro Giveaway"`
	// @Description Описание розыгрыша
	Description string `json:"description" example:"Разыгрываем новый iPhone 15 Pro!"`
	// @Description Время начала розыгрыша
	StartedAt string `json:"started_at" example:"2024-03-20T15:00:00Z"`
	// @Description Время окончания розыгрыша
	EndedAt string `json:"ended_at" example:"2024-03-21T15:00:00Z"`
	// @Description Длительность в секундах
	Duration int64 `json:"duration" example:"86400"`
	// @Description Максимальное количество участников
	MaxParticipants int `json:"max_participants" example:"1000"`
	// @Description Текущее количество участников
	ParticipantsCount int64 `json:"participants_count" example:"500"`
	// @Description Количество победителей
	WinnersCount int `json:"winners_count" example:"1"`
	// @Description Статус розыгрыша
	Status string `json:"status" example:"active" enums:"active,pending,completed,history,cancelled"`
	// @Description Роль пользователя
	UserRole string `json:"user_role" example:"participant" enums:"creator,participant,winner,viewer"`
	// @Description Количество билетов пользователя
	UserTickets int `json:"user_tickets" example:"5"`
	// @Description Общее количество билетов
	TotalTickets int `json:"total_tickets" example:"1000"`
	// @Description Список победителей с деталями
	Winners []WinnerDetail `json:"winners,omitempty"`
	// @Description Список призов с деталями
	Prizes []PrizeDetail `json:"prizes"`
}

// @Description Ответ с ошибкой
type ErrorResponse struct {
	// @Description Сообщение об ошибке
	Error string `json:"error" example:"Неверный ID розыгрыша"`
}

// @Description Ответ об успешном выполнении операции
type SuccessResponse struct {
	// @Description Сообщение об успехе
	Message string `json:"message" example:"Операция выполнена успешно"`
}

// @Description Ответ с отладочной информацией
type DebugResponse struct {
	// @Description Сообщение об ошибке
	Error string `json:"error" example:"Произошла ошибка"`
	// @Description Отладочная информация
	DebugInfo string `json:"debug_info,omitempty" example:"stack trace..."`
}

// @Description Тип приза
type SwaggerPrizeType string

// @Description Приз для определенного места
type SwaggerPrizePlace struct {
	// @Description ID приза
	PrizeID string `json:"prize_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// @Description Место (1 - первое, 2 - второе и т.д.)
	Place int `json:"place" example:"1" minimum:"1"`
	// @Description Тип приза (telegram_stars - звезды, telegram_premium - премиум, telegram_gifts - подарки, telegram_stickers - стикеры, custom - пользовательский)
	PrizeType string `json:"prize_type" example:"telegram_premium" enums:"telegram_stars,telegram_premium,telegram_gifts,telegram_stickers,custom"`
}

// @Description Детальная информация о призе
type SwaggerPrizeDetail struct {
	// @Description Тип приза
	Type PrizeType `json:"type" example:"telegram_premium"`
	// @Description Название приза
	Name string `json:"name" example:"Telegram Premium на 1 месяц"`
	// @Description Описание приза
	Description string `json:"description" example:"Подписка Telegram Premium на 1 месяц"`
	// @Description Является ли приз внутренним (выдается автоматически)
	IsInternal bool `json:"is_internal" example:"true"`
	// @Description Статус приза (pending - ожидает выдачи, distributed - выдан, cancelled - отменен)
	Status string `json:"status" example:"pending" enums:"pending,distributed,cancelled"`
}

// @Description Информация о победителе
type SwaggerWinnerDetail struct {
	// @Description ID пользователя
	UserID int64 `json:"user_id" example:"123456789"`
	// @Description Имя пользователя
	Username string `json:"username" example:"johndoe"`
	// @Description Занятое место
	Place int `json:"place" example:"1"`
	// @Description Информация о призе
	Prize PrizeDetail `json:"prize"`
	// @Description Время получения приза
	ReceivedAt string `json:"received_at,omitempty" example:"2024-03-21T15:00:00Z"`
}

// @Description Данные для создания пользовательского приза
type SwaggerCustomPrizeCreate struct {
	// @Description Название приза
	Name string `json:"name" binding:"required" example:"iPhone 15 Pro"`
	// @Description Описание приза
	Description string `json:"description" binding:"required" example:"Новый iPhone 15 Pro 256GB"`
}

// @Description Запрос на добавление билетов
type SwaggerTicketGrant struct {
	// @Description Количество билетов для добавления
	Count int64 `json:"count" binding:"required" minimum:"1" example:"5"`
}
