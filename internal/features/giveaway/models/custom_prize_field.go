package models

// CustomPrizeField представляет поле кастомного приза
type CustomPrizeField struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Type        string `json:"type" binding:"required,oneof=text number url email"`
	Value       string `json:"value" binding:"required"`
	Description string `json:"description,omitempty"`
}
