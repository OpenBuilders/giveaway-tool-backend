package models

import "time"

// User представляет полную модель пользователя в системе
// @Description Полная модель пользователя
type User struct {
	ID        int64     `json:"id" example:"123456789" description:"ID пользователя в Telegram"`
	Username  string    `json:"username" example:"johndoe" description:"Имя пользователя в Telegram"`
	FirstName string    `json:"first_name" example:"John" description:"Имя пользователя"`
	LastName  string    `json:"last_name" example:"Doe" description:"Фамилия пользователя"`
	Role      string    `json:"role" example:"user" enums:"user,admin" description:"Роль пользователя в системе"`
	Status    string    `json:"status" example:"active" enums:"active,banned" description:"Статус пользователя"`
	CreatedAt time.Time `json:"created_at" example:"2024-03-15T14:30:00Z" description:"Дата создания"`
	UpdatedAt time.Time `json:"updated_at" example:"2024-03-15T14:30:00Z" description:"Дата последнего обновления"`
}

// UserResponse представляет публичную информацию о пользователе
// @Description Публичная информация о пользователе
type UserResponse struct {
	ID        int64     `json:"id" example:"123456789" description:"ID пользователя в Telegram"`
	Username  string    `json:"username" example:"johndoe" description:"Имя пользователя в Telegram"`
	FirstName string    `json:"first_name" example:"John" description:"Имя пользователя"`
	LastName  string    `json:"last_name" example:"Doe" description:"Фамилия пользователя"`
	Role      string    `json:"role" example:"user" enums:"user,admin" description:"Роль пользователя в системе"`
	Status    string    `json:"status" example:"active" enums:"active,banned" description:"Статус пользователя"`
	CreatedAt time.Time `json:"created_at" example:"2024-03-15T14:30:00Z" description:"Дата создания"`
}
