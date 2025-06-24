package validation

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// Максимальные длины для различных полей
	MaxTitleLength       = 200
	MaxDescriptionLength = 1000
	MaxUsernameLength    = 32
	MaxFirstNameLength   = 64
	MaxLastNameLength    = 64
	MaxStatusLength      = 20
	MaxRoleLength        = 20

	// Минимальные длины
	MinTitleLength       = 1
	MinDescriptionLength = 1
	MinUsernameLength    = 1
	MinFirstNameLength   = 1
	MinLastNameLength    = 1
)

// Telegram username regex (допускает буквы, цифры, подчеркивания, 5-32 символа)
var telegramUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{5,32}$`)

// ValidateTitle проверяет заголовок
func ValidateTitle(title string) error {
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}

	title = strings.TrimSpace(title)
	if len(title) < MinTitleLength {
		return fmt.Errorf("title must be at least %d characters long", MinTitleLength)
	}

	if len(title) > MaxTitleLength {
		return fmt.Errorf("title cannot exceed %d characters", MaxTitleLength)
	}

	return nil
}

// ValidateDescription проверяет описание
func ValidateDescription(description string) error {
	if description == "" {
		return fmt.Errorf("description cannot be empty")
	}

	description = strings.TrimSpace(description)
	if len(description) < MinDescriptionLength {
		return fmt.Errorf("description must be at least %d characters long", MinDescriptionLength)
	}

	if len(description) > MaxDescriptionLength {
		return fmt.Errorf("description cannot exceed %d characters", MaxDescriptionLength)
	}

	return nil
}

// ValidateUsername проверяет Telegram username
func ValidateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	username = strings.TrimSpace(username)
	if len(username) < MinUsernameLength {
		return fmt.Errorf("username must be at least %d characters long", MinUsernameLength)
	}

	if len(username) > MaxUsernameLength {
		return fmt.Errorf("username cannot exceed %d characters", MaxUsernameLength)
	}

	// Убираем @ если есть
	if strings.HasPrefix(username, "@") {
		username = username[1:]
	}

	if !telegramUsernameRegex.MatchString(username) {
		return fmt.Errorf("username must contain only letters, numbers, and underscores, 5-32 characters")
	}

	return nil
}

// ValidateFirstName проверяет имя пользователя
func ValidateFirstName(firstName string) error {
	if firstName == "" {
		return fmt.Errorf("first name cannot be empty")
	}

	firstName = strings.TrimSpace(firstName)
	if len(firstName) < MinFirstNameLength {
		return fmt.Errorf("first name must be at least %d characters long", MinFirstNameLength)
	}

	if len(firstName) > MaxFirstNameLength {
		return fmt.Errorf("first name cannot exceed %d characters", MaxFirstNameLength)
	}

	return nil
}

// ValidateLastName проверяет фамилию пользователя
func ValidateLastName(lastName string) error {
	if lastName == "" {
		return fmt.Errorf("last name cannot be empty")
	}

	lastName = strings.TrimSpace(lastName)
	if len(lastName) < MinLastNameLength {
		return fmt.Errorf("last name must be at least %d characters long", MinLastNameLength)
	}

	if len(lastName) > MaxLastNameLength {
		return fmt.Errorf("last name cannot exceed %d characters", MaxLastNameLength)
	}

	return nil
}

// ValidateUserStatus проверяет статус пользователя
func ValidateUserStatus(status string) error {
	if status == "" {
		return fmt.Errorf("status cannot be empty")
	}

	status = strings.TrimSpace(status)
	if len(status) > MaxStatusLength {
		return fmt.Errorf("status cannot exceed %d characters", MaxStatusLength)
	}

	validStatuses := []string{"active", "banned", "inactive", "pending"}
	for _, validStatus := range validStatuses {
		if status == validStatus {
			return nil
		}
	}

	return fmt.Errorf("invalid status: %s. Valid statuses: %v", status, validStatuses)
}

// ValidateUserRole проверяет роль пользователя
func ValidateUserRole(role string) error {
	if role == "" {
		return fmt.Errorf("role cannot be empty")
	}

	role = strings.TrimSpace(role)
	if len(role) > MaxRoleLength {
		return fmt.Errorf("role cannot exceed %d characters", MaxRoleLength)
	}

	validRoles := []string{"user", "admin", "moderator"}
	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}

	return fmt.Errorf("invalid role: %s. Valid roles: %v", role, validRoles)
}

// ValidateChannelUsername проверяет username канала
func ValidateChannelUsername(username string) error {
	return ValidateUsername(username)
}

// ValidatePositiveInt проверяет, что число положительное
func ValidatePositiveInt(value int64, fieldName string) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", fieldName)
	}
	return nil
}

// ValidateNonNegativeInt проверяет, что число неотрицательное
func ValidateNonNegativeInt(value int64, fieldName string) error {
	if value < 0 {
		return fmt.Errorf("%s cannot be negative", fieldName)
	}
	return nil
}

// IsValidUsername проверяет валидность username
func IsValidUsername(username string) bool {
	if username == "" {
		return false
	}

	// Username должен быть от 5 до 32 символов
	if len(username) < 5 || len(username) > 32 {
		return false
	}

	// Username должен начинаться с буквы и содержать только буквы, цифры и подчеркивания
	usernameRegex := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
	return usernameRegex.MatchString(username)
}

// IsValidName проверяет валидность имени или фамилии
func IsValidName(name string) bool {
	if name == "" {
		return false
	}

	// Имя должно быть от 1 до 64 символов
	if len(name) < 1 || len(name) > 64 {
		return false
	}

	// Имя должно содержать только буквы, пробелы, дефисы и апострофы
	nameRegex := regexp.MustCompile(`^[a-zA-Zа-яА-Я\s\-']+$`)
	return nameRegex.MatchString(name)
}

// IsValidUserStatus проверяет валидность статуса пользователя
func IsValidUserStatus(status string) bool {
	validStatuses := []string{"active", "inactive", "banned"}
	for _, validStatus := range validStatuses {
		if status == validStatus {
			return true
		}
	}
	return false
}

// IsValidChannelTitle проверяет валидность названия канала
func IsValidChannelTitle(title string) bool {
	if title == "" {
		return false
	}

	// Название должно быть от 1 до 255 символов
	if len(title) < 1 || len(title) > 255 {
		return false
	}

	// Название не должно содержать только пробелы
	if strings.TrimSpace(title) == "" {
		return false
	}

	return true
}

// IsValidChannelUsername проверяет валидность username канала
func IsValidChannelUsername(username string) bool {
	if username == "" {
		return false
	}

	// Username должен быть от 5 до 32 символов
	if len(username) < 5 || len(username) > 32 {
		return false
	}

	// Username должен начинаться с буквы и содержать только буквы, цифры и подчеркивания
	usernameRegex := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
	return usernameRegex.MatchString(username)
}

// IsValidChannelStatus проверяет валидность статуса канала
func IsValidChannelStatus(status string) bool {
	validStatuses := []string{"active", "inactive", "banned"}
	for _, validStatus := range validStatuses {
		if status == validStatus {
			return true
		}
	}
	return false
}
