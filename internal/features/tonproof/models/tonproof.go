package models

import "time"

// TONProofRequest represents a request for TON Proof verification
// @Description Request for TON Proof verification
type TONProofRequest struct {
	Address     string `json:"address" binding:"required" example:"EQD4FPq-PRD4YtG87wgL7AErgQwHUMFQ-JxyYw8jzBPhqjfH"` // TON адрес кошелька
	Network     string `json:"network" binding:"required" example:"mainnet"`                                          // Сеть (mainnet/testnet)
	Proof       string `json:"proof" binding:"required" example:"base64_encoded_signature"`                           // Подпись сообщения
	State       string `json:"state" binding:"required" example:"random_state_string"`                                // Случайная строка состояния
	WalletState string `json:"wallet_state" binding:"required" example:"wallet_state"`                                // Состояние кошелька
	PublicKey   string `json:"public_key" binding:"required" example:"base64_encoded_public_key"`                     // Публичный ключ в формате base64
}

// TONProofResponse represents a response for the verification request
// @Description Response for TON Proof verification
type TONProofResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// TONProofRecord represents a record of the verification
// @Description Record of TON Proof verification
type TONProofRecord struct {
	UserID      int64     `json:"user_id"`
	Address     string    `json:"address"`
	Network     string    `json:"network"`
	VerifiedAt  time.Time `json:"verified_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsValid     bool      `json:"is_valid"`
	LastChecked time.Time `json:"last_checked"`
}
