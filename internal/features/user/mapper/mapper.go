package mapper

import "giveaway-tool-backend/internal/features/user/models"

// ToUserResponse maps User model to UserResponse DTO
func ToUserResponse(user *models.User) *models.UserResponse {
	return &models.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	}
}
