package models

import "errors"

var (
	ErrTicketsNotAllowed  = errors.New("tickets are not allowed for this giveaway")
	ErrInvalidTicketCount = errors.New("invalid ticket count")
)

// TicketGrant представляет запрос на выдачу билетов
type TicketGrant struct {
	UserID      int64 `json:"user_id" binding:"required"`
	TicketCount int64 `json:"ticket_count" binding:"required,min=1"`
}
