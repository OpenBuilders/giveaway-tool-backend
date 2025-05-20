package models

import "time"

// TONProofState представляет состояние для верификации TON Proof
type TONProofState struct {
	UserID    int64     `json:"user_id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

// TONProofRequest представляет запрос на верификацию TON Proof
type TONProofRequest struct {
	Address     string `json:"address"`
	Network     string `json:"network"`
	Proof       Proof  `json:"proof"`
	WalletState struct {
		PublicKey string `json:"publicKey"`
	} `json:"walletState"`
}

// Proof представляет данные подписи TON Proof
type Proof struct {
	Timestamp string `json:"timestamp"`
	Domain    struct {
		LengthBytes int    `json:"lengthBytes"`
		Value       string `json:"value"`
	} `json:"domain"`
	Signature string `json:"signature"`
	Payload   string `json:"payload"`
}

type TONProofResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Successfully verified"`
}

// TONProofRecord представляет запись о верификации TON Proof
type TONProofRecord struct {
	UserID     int64     `json:"user_id"`
	Address    string    `json:"address"`
	Network    string    `json:"network"`
	VerifiedAt time.Time `json:"verified_at"`
}
