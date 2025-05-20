package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"giveaway-tool-backend/internal/features/tonproof/models"
	"giveaway-tool-backend/internal/features/tonproof/repository"
	"strconv"
	"time"
)

type Service struct {
	repo repository.Repository
}

func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GenerateState(ctx context.Context, userID int64) (*models.TONProofState, error) {
	return s.repo.GenerateState(ctx, userID)
}

func (s *Service) VerifyProof(ctx context.Context, userID int64, req *models.TONProofRequest) error {
	if req.Proof.Domain.Value != "ton.app" {
		return fmt.Errorf("invalid domain")
	}

	timestamp, err := strconv.ParseInt(req.Proof.Timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if time.Now().Unix() > timestamp+300 {
		return fmt.Errorf("proof expired")
	}

	if err := s.verifySignature(req); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	record := &models.TONProofRecord{
		UserID:     userID,
		Address:    req.Address,
		Network:    req.Network,
		VerifiedAt: time.Now(),
	}

	return s.repo.SaveProof(ctx, record)
}

func (s *Service) IsVerified(ctx context.Context, userID int64) (bool, error) {
	record, err := s.repo.GetProof(ctx, userID)
	if err != nil {
		return false, err
	}
	return record != nil, nil
}

func (s *Service) verifySignature(req *models.TONProofRequest) error {
	message := fmt.Sprintf("%s:%s:%s", req.Proof.Domain.Value, req.Proof.Timestamp, req.Proof.Payload)

	pubKey, err := base64.StdEncoding.DecodeString(req.WalletState.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}

	signature, err := base64.StdEncoding.DecodeString(req.Proof.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	if !ed25519.Verify(pubKey, []byte(message), signature) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
