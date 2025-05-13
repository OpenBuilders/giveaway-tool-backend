package model

type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	IsPremium bool   `json:"is_premium"`
	Avatar    string `json:"avatar"`
	CreatedAt string `json:"created_at"`
}
