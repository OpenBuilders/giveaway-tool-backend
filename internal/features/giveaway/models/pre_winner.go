package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// PreWinnerUser представляет пользователя из pre-winner list
type PreWinnerUser struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

// PreWinnerListRequest представляет запрос на загрузку pre-winner list
type PreWinnerListRequest struct {
	GiveawayID string  `json:"giveaway_id" binding:"required"`
	UserIDs    []int64 `json:"user_ids" binding:"required"`
}

// PreWinnerListResponse представляет ответ с обработанным pre-winner list
type PreWinnerListResponse struct {
	GiveawayID    string          `json:"giveaway_id"`
	TotalUploaded int             `json:"total_uploaded"` // Общее количество загруженных ID
	ValidUsers    []PreWinnerUser `json:"valid_users"`    // Пользователи, прошедшие все проверки
	InvalidUsers  []int64         `json:"invalid_users"`  // ID пользователей, не прошедших проверки
	Message       string          `json:"message"`        // Сообщение о результате
}

// PreWinnerListStored представляет сохраненный pre-winner list в Redis
type PreWinnerListStored struct {
	GiveawayID string          `json:"giveaway_id"`
	UserIDs    []int64         `json:"user_ids"`   // Только ID пользователей
	Users      []PreWinnerUser `json:"users"`      // Полная информация о пользователях
	CreatedAt  int64           `json:"created_at"` // Unix timestamp
}

// ParseUserIDsFromFile парсит user_id из текстового файла
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

			// Избегаем дубликатов
			if !seenIDs[userID] {
				seenIDs[userID] = true
				userIDs = append(userIDs, userID)
			}
		}
	}

	return userIDs, nil
}

// ValidatePreWinnerList проверяет корректность pre-winner list
func ValidatePreWinnerList(giveawayID string, userIDs []int64) error {
	if giveawayID == "" {
		return fmt.Errorf("giveaway_id is required")
	}

	if len(userIDs) == 0 {
		return fmt.Errorf("at least one user_id is required")
	}

	// Проверяем, что все ID положительные
	for _, userID := range userIDs {
		if userID <= 0 {
			return fmt.Errorf("invalid user_id: %d", userID)
		}
	}

	return nil
}

// PreWinnerValidationResult представляет результат валидации пользователя
type PreWinnerValidationResult struct {
	UserID            int64  `json:"user_id"`
	IsParticipant     bool   `json:"is_participant"`     // Участвует ли в гиве
	MeetsRequirements bool   `json:"meets_requirements"` // Выполняет ли все требования
	Error             string `json:"error,omitempty"`    // Ошибка валидации
}

// PreWinnerValidationResponse представляет ответ валидации
type PreWinnerValidationResponse struct {
	GiveawayID string                      `json:"giveaway_id"`
	Results    []PreWinnerValidationResult `json:"results"`
	ValidCount int                         `json:"valid_count"`
	TotalCount int                         `json:"total_count"`
}

// CompleteWithCustomResponse представляет ответ завершения гива с Custom требованиями
type CompleteWithCustomResponse struct {
	GiveawayID   string          `json:"giveaway_id"`
	WinnersCount int             `json:"winners_count"`
	Winners      []PreWinnerUser `json:"winners"`
	Message      string          `json:"message"`
}

// MarshalJSON для PreWinnerUser
func (p *PreWinnerUser) MarshalJSON() ([]byte, error) {
	type Alias PreWinnerUser
	return json.Marshal(&struct {
		*Alias
		UserID string `json:"user_id"` // Представляем как строку для совместимости
	}{
		Alias:  (*Alias)(p),
		UserID: strconv.FormatInt(p.UserID, 10),
	})
}

// UnmarshalJSON для PreWinnerUser
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

	// Парсим user_id из строки
	userID, err := strconv.ParseInt(aux.UserID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid user_id format: %s", aux.UserID)
	}

	p.UserID = userID
	return nil
}
