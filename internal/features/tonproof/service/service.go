package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"giveaway-tool-backend/internal/features/tonproof/models"
	"giveaway-tool-backend/internal/features/tonproof/repository"
	"time"

	"github.com/xssnick/tonutils-go/address"
)

type Service interface {
	VerifyProof(ctx context.Context, userID int64, req *models.TONProofRequest) (*models.TONProofResponse, error)
	IsVerified(ctx context.Context, userID int64) (bool, error)
}

type service struct {
	repo repository.Repository
}

func NewService(repo repository.Repository) Service {
	return &service{repo: repo}
}

func (s *service) VerifyProof(ctx context.Context, userID int64, req *models.TONProofRequest) (*models.TONProofResponse, error) {
	if err := s.verifySignature(req); err != nil {
		return &models.TONProofResponse{
			Success: false,
			Message: fmt.Sprintf("Invalid signature: %v", err),
		}, nil
	}

	proof := &models.TONProofRecord{
		UserID:      userID,
		Address:     req.Address,
		Network:     req.Network,
		VerifiedAt:  time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		IsValid:     true,
		LastChecked: time.Now(),
	}

	if err := s.repo.SaveProof(ctx, userID, proof); err != nil {
		return nil, fmt.Errorf("failed to save proof: %w", err)
	}

	return &models.TONProofResponse{
		Success: true,
		Message: "TON Proof verified successfully",
	}, nil
}

func (s *service) IsVerified(ctx context.Context, userID int64) (bool, error) {
	return s.repo.IsProofValid(ctx, userID)
}

func (s *service) verifySignature(req *models.TONProofRequest) error {
	signature, err := base64.StdEncoding.DecodeString(req.Proof)
	if err != nil {
		return fmt.Errorf("invalid proof format: %w", err)
	}

	message := fmt.Sprintf("%s:%s:%s", req.Address, req.Network, req.State)
	hash := sha256.Sum256([]byte(message))

	publicKey, err := s.getPublicKeyFromAddress(req.Address)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	if !ed25519.Verify(publicKey, hash[:], signature) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

func (s *service) getPublicKeyFromAddress(addrStr string) (ed25519.PublicKey, error) {
	addr, err := address.ParseAddr(addrStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TON address: %w", err)
	}

	key, err := base64.StdEncoding.DecodeString(addr.String())
	if err != nil {
		return nil, fmt.Errorf("failed to decode address: %w", err)
	}

	if len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length: expected %d, got %d", ed25519.PublicKeySize, len(key))
	}

	return key, nil
}
