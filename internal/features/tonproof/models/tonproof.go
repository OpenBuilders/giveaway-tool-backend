package models

import "time"

// TONProofRequest represents a request for TON Proof verification
// @Description Request for TON Proof verification
type TONProofRequest struct {
	Address     string `json:"address" binding:"required"`
	Network     string `json:"network" binding:"required"`
	Proof       string `json:"proof" binding:"required"`
	State       string `json:"state" binding:"required"`
	WalletState string `json:"wallet_state" binding:"required"`
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
