package models

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrGiveawayNotEditable    = errors.New("giveaway can only be edited within first 5 minutes")
	ErrMaxParticipantsReached = errors.New("maximum number of participants reached")
	ErrGiveawayEnded          = errors.New("giveaway has ended")
	ErrInvalidWinnersCount    = errors.New("winners count must be greater than 0 and less than max participants")
)

// GiveawayStatus represents the status of a giveaway
type GiveawayStatus string

const (
	GiveawayStatusActive     GiveawayStatus = "active"  // Active giveaway
	GiveawayStatusPending    GiveawayStatus = "pending" // Pending processing (selecting winners)
	GiveawayStatusProcessing GiveawayStatus = "processing"
	GiveawayStatusCompleted  GiveawayStatus = "completed" // Completed, winners selected
	GiveawayStatusHistory    GiveawayStatus = "history"   // In history (all prizes distributed)
	GiveawayStatusCancelled  GiveawayStatus = "cancelled" // Cancelled
)

// Giveaway represents a giveaway event
type Giveaway struct {
	ID              string         `json:"id"`
	CreatorID       int64          `json:"creator_id"`
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	StartedAt       time.Time      `json:"started_at"`
	EndsAt          time.Time      `json:"ends_at"`
	Duration        int64          `json:"duration"`                   // in seconds
	MaxParticipants int            `json:"max_participants,omitempty"` // 0 = unlimited
	WinnersCount    int            `json:"winners_count"`
	Status          GiveawayStatus `json:"status"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	Prizes          []PrizePlace   `json:"prizes,omitempty"`
	AutoDistribute  bool           `json:"auto_distribute,omitempty"` // Automatic prize distribution
	AllowTickets    bool           `json:"allow_tickets"`             // Whether tickets are allowed
	Requirements    []Requirement  `json:"requirements"`              // Participation requirements
	MsgID           int64          `json:"msg_id"`
}

// UnmarshalJSON implements json.Unmarshaler
func (g *Giveaway) UnmarshalJSON(data []byte) error {
	// Сначала пробуем обычную структуру без Requirements
	type RawGiveaway struct {
		ID              string         `json:"id"`
		CreatorID       int64          `json:"creator_id"`
		Title           string         `json:"title"`
		Description     string         `json:"description"`
		StartedAt       time.Time      `json:"started_at"`
		EndsAt          time.Time      `json:"ends_at"`
		Duration        int64          `json:"duration"`
		MaxParticipants int            `json:"max_participants,omitempty"`
		WinnersCount    int            `json:"winners_count"`
		Status          GiveawayStatus `json:"status"`
		CreatedAt       time.Time      `json:"created_at"`
		UpdatedAt       time.Time      `json:"updated_at"`
		Prizes          []PrizePlace   `json:"prizes,omitempty"`
		AutoDistribute  bool           `json:"auto_distribute,omitempty"`
		AllowTickets    bool           `json:"allow_tickets"`
		MsgID           int64          `json:"msg_id"`
	}

	raw := &RawGiveaway{}
	if err := json.Unmarshal(data, raw); err != nil {
		return err
	}

	// Копируем базовые поля
	g.ID = raw.ID
	g.CreatorID = raw.CreatorID
	g.Title = raw.Title
	g.Description = raw.Description
	g.StartedAt = raw.StartedAt
	g.EndsAt = raw.EndsAt
	g.Duration = raw.Duration
	g.MaxParticipants = raw.MaxParticipants
	g.WinnersCount = raw.WinnersCount
	g.Status = raw.Status
	g.CreatedAt = raw.CreatedAt
	g.UpdatedAt = raw.UpdatedAt
	g.Prizes = raw.Prizes
	g.AutoDistribute = raw.AutoDistribute
	g.AllowTickets = raw.AllowTickets
	g.MsgID = raw.MsgID

	// Теперь пробуем получить requirements из разных возможных форматов
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// Проверяем наличие поля requirements
	if reqData, ok := rawMap["requirements"]; ok {
		var reqs []Requirement
		if err := json.Unmarshal(reqData, &reqs); err == nil {
			g.Requirements = reqs
		} else {
			// Если не удалось распарсить как массив, пробуем как объект старого формата
			var oldReqs struct {
				Requirements []Requirement `json:"requirements"`
				Enabled      bool          `json:"enabled"`
			}
			if err := json.Unmarshal(reqData, &oldReqs); err == nil {
				g.Requirements = oldReqs.Requirements
			} else {
				// Если и это не удалось, просто оставляем пустой массив
				g.Requirements = make([]Requirement, 0)
			}
		}
	} else {
		// Если поля нет вообще, инициализируем пустым массивом
		g.Requirements = make([]Requirement, 0)
	}

	return nil
}

// MarshalJSON implements json.Marshaler
func (g *Giveaway) MarshalJSON() ([]byte, error) {
	type Alias Giveaway
	return json.Marshal(&struct {
		*Alias
		Requirements []Requirement `json:"requirements,omitempty"`
	}{
		Alias:        (*Alias)(g),
		Requirements: g.Requirements,
	})
}

// Winner represents a giveaway winner
type Winner struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Place    int    `json:"place"`
}

// GiveawayCreate represents data for creating a new giveaway
type GiveawayCreate struct {
	Title           string        `json:"title" binding:"required,min=3,max=100"`
	Description     string        `json:"description" binding:"required,min=10,max=1000"`
	Duration        int64         `json:"duration" binding:"required,min=5"`
	MaxParticipants int           `json:"max_participants" binding:"min=0"`
	WinnersCount    int           `json:"winners_count" binding:"required,min=1"`
	Prizes          []PrizePlace  `json:"prizes" binding:"required"`
	AutoDistribute  bool          `json:"auto_distribute"`
	AllowTickets    bool          `json:"allow_tickets"`
	Requirements    []Requirement `json:"requirements"`
}

type GiveawayUpdate struct {
	Title       *string      `json:"title,omitempty" binding:"omitempty,min=3,max=100"`
	Description *string      `json:"description,omitempty" binding:"omitempty,min=10,max=1000"`
	Prizes      []PrizePlace `json:"prizes,omitempty" binding:"omitempty,dive"`
}

type GiveawayResponse struct {
	ID                string         `json:"id"`
	CreatorID         int64          `json:"creator_id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	StartedAt         time.Time      `json:"started_at"`
	EndsAt            time.Time      `json:"ends_at"`
	MaxParticipants   int            `json:"max_participants,omitempty"`
	WinnersCount      int            `json:"winners_count"`
	Status            GiveawayStatus `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	ParticipantsCount int64          `json:"participants_count"`
	CanEdit           bool           `json:"can_edit"`
	UserRole          string         `json:"user_role"` // owner, participant, user
	Prizes            []PrizePlace   `json:"prizes,omitempty"`
	Requirements      []Requirement  `json:"requirements"`
	AutoDistribute    bool           `json:"auto_distribute,omitempty"`
	Winners           []Winner       `json:"winners,omitempty"` // Только для завершенных розыгрышей
	AllowTickets      bool           `json:"allow_tickets"`     // Разрешены ли билеты
	MsgID             int64          `json:"msg_id"`
}

// UnmarshalJSON implements json.Unmarshaler
func (g *GiveawayResponse) UnmarshalJSON(data []byte) error {
	// Сначала пробуем обычную структуру без Requirements
	type RawGiveawayResponse struct {
		ID                string         `json:"id"`
		CreatorID         int64          `json:"creator_id"`
		Title             string         `json:"title"`
		Description       string         `json:"description"`
		StartedAt         time.Time      `json:"started_at"`
		EndsAt            time.Time      `json:"ends_at"`
		MaxParticipants   int            `json:"max_participants,omitempty"`
		WinnersCount      int            `json:"winners_count"`
		Status            GiveawayStatus `json:"status"`
		CreatedAt         time.Time      `json:"created_at"`
		UpdatedAt         time.Time      `json:"updated_at"`
		ParticipantsCount int64          `json:"participants_count"`
		CanEdit           bool           `json:"can_edit"`
		UserRole          string         `json:"user_role"`
		Prizes            []PrizePlace   `json:"prizes,omitempty"`
		AutoDistribute    bool           `json:"auto_distribute,omitempty"`
		Winners           []Winner       `json:"winners,omitempty"`
		AllowTickets      bool           `json:"allow_tickets"`
		MsgID             int64          `json:"msg_id"`
	}

	raw := &RawGiveawayResponse{}
	if err := json.Unmarshal(data, raw); err != nil {
		return err
	}

	// Копируем базовые поля
	g.ID = raw.ID
	g.CreatorID = raw.CreatorID
	g.Title = raw.Title
	g.Description = raw.Description
	g.StartedAt = raw.StartedAt
	g.EndsAt = raw.EndsAt
	g.MaxParticipants = raw.MaxParticipants
	g.WinnersCount = raw.WinnersCount
	g.Status = raw.Status
	g.CreatedAt = raw.CreatedAt
	g.UpdatedAt = raw.UpdatedAt
	g.ParticipantsCount = raw.ParticipantsCount
	g.CanEdit = raw.CanEdit
	g.UserRole = raw.UserRole
	g.Prizes = raw.Prizes
	g.AutoDistribute = raw.AutoDistribute
	g.Winners = raw.Winners
	g.AllowTickets = raw.AllowTickets
	g.MsgID = raw.MsgID

	// Теперь пробуем получить requirements из разных возможных форматов
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// Проверяем наличие поля requirements
	if reqData, ok := rawMap["requirements"]; ok {
		var reqs []Requirement
		if err := json.Unmarshal(reqData, &reqs); err == nil {
			g.Requirements = reqs
		} else {
			// Если не удалось распарсить как массив, пробуем как объект старого формата
			var oldReqs struct {
				Requirements []Requirement `json:"requirements"`
				Enabled      bool          `json:"enabled"`
			}
			if err := json.Unmarshal(reqData, &oldReqs); err == nil {
				g.Requirements = oldReqs.Requirements
			} else {
				// Если и это не удалось, просто оставляем пустой массив
				g.Requirements = make([]Requirement, 0)
			}
		}
	} else {
		// Если поля нет вообще, инициализируем пустым массивом
		g.Requirements = make([]Requirement, 0)
	}

	return nil
}

// MarshalJSON implements json.Marshaler
func (g *GiveawayResponse) MarshalJSON() ([]byte, error) {
	type Alias GiveawayResponse
	return json.Marshal(&struct {
		*Alias
		Requirements []Requirement `json:"requirements,omitempty"`
	}{
		Alias:        (*Alias)(g),
		Requirements: g.Requirements,
	})
}

// GiveawayDetailedResponse represents detailed information about a giveaway
type GiveawayDetailedResponse struct {
	ID                string         `json:"id"`
	CreatorID         int64          `json:"creator_id"`
	CreatorUsername   string         `json:"creator_username"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	StartedAt         time.Time      `json:"started_at"`
	EndsAt            time.Time      `json:"ends_at"`
	Duration          int64          `json:"duration"` // in seconds
	MaxParticipants   int            `json:"max_participants"`
	ParticipantsCount int64          `json:"participants_count"`
	WinnersCount      int            `json:"winners_count"`
	Status            GiveawayStatus `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Winners           []WinnerDetail `json:"winners,omitempty"`
	Prizes            []PrizeDetail  `json:"prizes"`
	UserRole          string         `json:"user_role"`     // owner, participant, winner
	UserTickets       int            `json:"user_tickets"`  // количество билетов пользователя
	TotalTickets      int            `json:"total_tickets"` // общее количество билетов
}

// WinnerDetail represents detailed information about a winner
type WinnerDetail struct {
	UserID     int64       `json:"user_id"`
	Username   string      `json:"username"`
	Place      int         `json:"place"`
	Prize      PrizeDetail `json:"prize"`
	ReceivedAt time.Time   `json:"received_at,omitempty"`
}

// PrizeDetail represents detailed information about a prize
type PrizeDetail struct {
	Type        PrizeType `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsInternal  bool      `json:"is_internal"`
	Status      string    `json:"status"` // pending, distributed, cancelled
}

func (g *Giveaway) IsEditable() bool {
	return time.Since(g.StartedAt) <= 5*time.Minute
}

func (g *Giveaway) HasEnded() bool {
	now := time.Now()
	if g.StartedAt.After(now) {
		return false
	}
	return now.Sub(g.StartedAt) > time.Duration(g.Duration)*time.Second
}

func (g *Giveaway) CanAddParticipant() bool {
	if g.HasEnded() {
		return false
	}
	if g.Status != "active" {
		return false
	}
	if g.MaxParticipants > 0 {
		// Получаем количество участников из отдельного счетчика в Redis
		return false // Это будет реализовано в репозитории
	}
	return true
}

const (
	MinDurationDebug           int64 = 5   // 5 seconds for debug mode
	MinDurationRelease         int64 = 300 // 5 minutes for release mode
	MaxCancellationTimeMinutes       = 5   // Максимальное время для отмены гивевея в минутах
	MaxParticipantsForCancel         = 10  // Максимальное количество участников для отмены
)

// GiveawayStatusUpdate represents a giveaway status update
type GiveawayStatusUpdate struct {
	GiveawayID string
	OldStatus  GiveawayStatus
	NewStatus  GiveawayStatus
	UpdatedAt  time.Time
	Reason     string
}
