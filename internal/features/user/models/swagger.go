package models

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error" example:"Error message"`
}

// StatusUpdate represents a status update request
type StatusUpdate struct {
	Status string `json:"status" example:"active" enums:"active,banned"`
}

// UsersResponse represents a paginated list of users
type UsersResponse struct {
	Items []UserResponse `json:"items"`
	Total int            `json:"total" example:"42"`
}
