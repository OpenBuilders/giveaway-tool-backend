package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type PreWinnerUser struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

type PreWinnerListRequest struct {
	GiveawayID string  `json:"giveaway_id" binding:"required"`
	UserIDs    []int64 `json:"user_ids" binding:"required"`
}

type PreWinnerListResponse struct {
	GiveawayID    string          `json:"giveaway_id"`
	TotalUploaded int             `json:"total_uploaded"`
	ValidUsers    []PreWinnerUser `json:"valid_users"`
	InvalidUsers  []int64         `json:"invalid_users"`
	Message       string          `json:"message"`
}

type PreWinnerListStored struct {
	GiveawayID string          `json:"giveaway_id"`
	UserIDs    []int64         `json:"user_ids"`
	Users      []PreWinnerUser `json:"users"`
	CreatedAt  int64           `json:"created_at"`
}

func ParseUserIDsFromFile(content []byte) ([]int64, error) {
	text := string(content)
	lines := strings.Split(text, "\n")

	var userIDs []int64
	seenIDs := make(map[int64]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Поддерживаем разные форматы: только ID, ID с запятой, ID с пробелами
		parts := strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})

		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			userID, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid user_id format: %s", part)
			}

			if !seenIDs[userID] {
				seenIDs[userID] = true
				userIDs = append(userIDs, userID)
			}
		}
	}

	return userIDs, nil
}

func ValidatePreWinnerList(giveawayID string, userIDs []int64) error {
	if giveawayID == "" {
		return fmt.Errorf("giveaway_id is required")
	}

	if len(userIDs) == 0 {
		return fmt.Errorf("at least one user_id is required")
	}

	for _, userID := range userIDs {
		if userID <= 0 {
			return fmt.Errorf("invalid user_id: %d", userID)
		}
	}

	return nil
}

type PreWinnerValidationResult struct {
	UserID            int64  `json:"user_id"`
	IsParticipant     bool   `json:"is_participant"`
	MeetsRequirements bool   `json:"meets_requirements"`
	Error             string `json:"error,omitempty"`
}

type PreWinnerValidationResponse struct {
	GiveawayID string                      `json:"giveaway_id"`
	Results    []PreWinnerValidationResult `json:"results"`
	ValidCount int                         `json:"valid_count"`
	TotalCount int                         `json:"total_count"`
}

type CompleteWithCustomResponse struct {
	GiveawayID   string          `json:"giveaway_id"`
	WinnersCount int             `json:"winners_count"`
	Winners      []PreWinnerUser `json:"winners"`
	Message      string          `json:"message"`
}

func (p *PreWinnerUser) MarshalJSON() ([]byte, error) {
	type Alias PreWinnerUser
	return json.Marshal(&struct {
		*Alias
		UserID string `json:"user_id"`
	}{
		Alias:  (*Alias)(p),
		UserID: strconv.FormatInt(p.UserID, 10),
	})
}

func (p *PreWinnerUser) UnmarshalJSON(data []byte) error {
	type Alias PreWinnerUser
	aux := &struct {
		*Alias
		UserID string `json:"user_id"`
	}{
		Alias: (*Alias)(p),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	userID, err := strconv.ParseInt(aux.UserID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid user_id format: %s", aux.UserID)
	}

	p.UserID = userID
	return nil
}
