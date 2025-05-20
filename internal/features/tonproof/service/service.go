package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"giveaway-tool-backend/internal/features/tonproof/models"
	"giveaway-tool-backend/internal/features/tonproof/repository"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tvm/cell"
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
	publicKeyBytes, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key format: %w", err)
	}

	if len(publicKeyBytes) != 32 {
		return fmt.Errorf("invalid public key length: expected 32 bytes, got %d", len(publicKeyBytes))
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(req.Proof)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	if len(signatureBytes) != 64 {
		return fmt.Errorf("invalid signature length: expected 64 bytes, got %d", len(signatureBytes))
	}

	message := fmt.Sprintf("%s:%s:%s", req.Address, req.Network, req.State)

	valid := ed25519.Verify(publicKeyBytes, []byte(message), signatureBytes)
	if !valid {
		return fmt.Errorf("invalid signature")
	}

	addr, err := address.ParseAddr(req.Address)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	stateCell := cell.BeginCell()
	stateCell.StoreSlice(publicKeyBytes, 256)
	stateCell.StoreUInt(0, 8)
	stateCell.StoreUInt(0, 1)

	stateHash := stateCell.EndCell().Hash()

	if !bytes.Equal(addr.Data(), stateHash) {
		return fmt.Errorf("public key does not match the address")
	}

	return nil
}
