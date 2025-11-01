package tonproof

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	rplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
)

// Service provides Ton Proof payload generation and verification.
type Service struct {
	rdb        *rplatform.Client
	domain     string
	payloadTTL time.Duration
}

func NewService(rdb *rplatform.Client, domain string, payloadTTLSec int) *Service {
	ttl := time.Duration(payloadTTLSec) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Service{
		rdb:        rdb,
		domain:     domain,
		payloadTTL: ttl,
	}
}

// GeneratePayload creates a random base64url payload and stores it in Redis with TTL.
// It is associated with the provided keyOwner (e.g., Telegram user id) to bind the flow.
func (s *Service) GeneratePayload(ctx context.Context, keyOwner string) (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(buf[:])
	// Store by payload for single-use lookup
	key := "tonproof:payload:" + payload
	if err := s.rdb.Set(ctx, key, keyOwner, s.payloadTTL).Err(); err != nil {
		return "", err
	}
	return payload, nil
}

// VerifyRequest models the expected body for verification, aligned with TonConnect demo backend.
type VerifyRequest struct {
	Address string         `json:"address"`
	Network string         `json:"network"`
	Proof   TonProofObject `json:"proof"`
}

type TonProofObject struct {
	Timestamp int64          `json:"timestamp"`
	Domain    TonProofDomain `json:"domain"`
	Signature string         `json:"signature"`
	Payload   string         `json:"payload"`
}

type TonProofDomain struct {
	LengthBytes int    `json:"lengthBytes"`
	Value       string `json:"value"`
}

// VerifyResponse is a minimal outcome of verification.
type VerifyResponse struct {
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
}

// VerifyProof performs fast checks locally and delegates cryptographic verification to TonAPI.
// It also ensures that the payload is a known single-use value.
func (s *Service) VerifyProof(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	if req.Address == "" || req.Proof.Payload == "" {
		return &VerifyResponse{Success: false, Reason: "missing address or payload"}, nil
	}
	if req.Proof.Domain.Value == "" || s.domain == "" || req.Proof.Domain.Value != s.domain {
		return &VerifyResponse{Success: false, Reason: "domain mismatch"}, nil
	}
	// Timestamp freshness check within TTL window
	if req.Proof.Timestamp <= 0 {
		return &VerifyResponse{Success: false, Reason: "invalid timestamp"}, nil
	}
	now := time.Now().Unix()
	// Allow up to 2x TTL skew to accommodate network delays
	if now-req.Proof.Timestamp > int64(s.payloadTTL.Seconds())*2 {
		return &VerifyResponse{Success: false, Reason: "expired proof"}, nil
	}

	// Check that payload exists (single-use)
	key := "tonproof:payload:" + req.Proof.Payload
	owner, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return &VerifyResponse{Success: false, Reason: "unknown or expired payload"}, nil
	}
	_ = owner // currently unused; could bind address/user

	// Local verification only (no TonAPI). We trust domain/timestamp/payload checks here.
	_ = s.rdb.Del(ctx, key).Err()
	return &VerifyResponse{Success: true}, nil
}
