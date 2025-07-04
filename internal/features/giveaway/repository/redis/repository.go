package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
	usermodels "giveaway-tool-backend/internal/features/user/models"
	"math/rand"
	"sort"
	"strconv"
	"time"

	"log"

	"bytes"

	"github.com/redis/go-redis/v9"
)

type redisTransaction struct {
	pipe redis.Pipeliner
}

func (tx *redisTransaction) Commit() error {
	_, err := tx.pipe.Exec(context.Background())
	return err
}

func (tx *redisTransaction) Rollback() error {
	tx.pipe.Discard()
	return nil
}

type redisRepository struct {
	client *redis.Client
}

const (
	keyPrefixGiveaway          = "giveaway:"
	keyPrefixPrize             = "prize:"
	keyPrefixParticipantsCount = "giveaway:participants_count:"
	keyActiveGiveaways         = "giveaways:active"
	keyPendingGiveaways        = "giveaways:pending"
	keyHistoryGiveaways        = "giveaways:history"
	keyPrizeTemplates          = "prize:templates"
	keyRequirementTemplates    = "requirement:templates"
	keyTopGiveaways            = "giveaways:top"
	defaultLockTimeout         = 30 * time.Second
	keyProcessingSet           = "giveaways:processing"
	keyRequirements            = "requirements"    // Префикс для ключей требований
	channelAvatarKey           = "channel:avatar:" // + channelID
)

func NewRedisGiveawayRepository(client *redis.Client) repository.GiveawayRepository {
	return &redisRepository{client: client}
}

func makeGiveawayKey(id string) string {
	return keyPrefixGiveaway + id
}

func makeParticipantsCountKey(id string) string {
	return keyPrefixParticipantsCount + id
}

func makePrizeKey(id string) string {
	return keyPrefixPrize + id
}

func makeRequirementsKey(giveawayID string) string {
	return fmt.Sprintf("giveaway:%s:%s", giveawayID, keyRequirements)
}

func (r *redisRepository) BeginTx(ctx context.Context) (repository.Transaction, error) {
	return &redisTransaction{
		pipe: r.client.Pipeline(),
	}, nil
}

func (r *redisRepository) Create(ctx context.Context, giveaway *models.Giveaway) error {
	// Создаем копию гивевея без требований для основного хранения
	giveawayData := *giveaway
	giveawayData.Requirements = nil

	data, err := json.Marshal(giveawayData)
	if err != nil {
		return fmt.Errorf("failed to marshal giveaway: %w", err)
	}

	pipe := r.client.Pipeline()

	// Сохраняем основные данные гивевея
	pipe.Set(ctx, makeGiveawayKey(giveaway.ID), data, 0)
	pipe.SAdd(ctx, keyActiveGiveaways, giveaway.ID)
	pipe.Set(ctx, makeParticipantsCountKey(giveaway.ID), 0, 0)

	// Если есть требования, сохраняем их отдельно
	if len(giveaway.Requirements) > 0 {
		reqData, err := json.Marshal(giveaway.Requirements)
		if err != nil {
			return fmt.Errorf("failed to marshal requirements: %w", err)
		}
		pipe.Set(ctx, makeRequirementsKey(giveaway.ID), reqData, 0)
	}

	_, err = pipe.Exec(ctx)
	return err
}

func (r *redisRepository) GetByID(ctx context.Context, id string) (*models.Giveaway, error) {
	// Получаем основные данные гивевея
	data, err := r.client.Get(ctx, makeGiveawayKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, repository.ErrGiveawayNotFound
		}
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	var giveaway models.Giveaway
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&giveaway); err != nil {
		return nil, fmt.Errorf("failed to unmarshal giveaway: %w", err)
	}

	// Получаем требования отдельно
	reqData, err := r.client.Get(ctx, makeRequirementsKey(id)).Bytes()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get requirements: %w", err)
	}

	if err != redis.Nil && len(reqData) > 0 {
		var requirements []models.Requirement
		if err := json.Unmarshal(reqData, &requirements); err != nil {
			return nil, fmt.Errorf("failed to unmarshal requirements: %w", err)
		}
		giveaway.Requirements = requirements
	}

	return &giveaway, nil
}

func (r *redisRepository) GetByIDWithLock(ctx context.Context, tx repository.Transaction, id string) (*models.Giveaway, error) {
	lockKey := makeGiveawayKey(id) + ":lock"

	// Пытаемся получить блокировку с таймаутом
	ok, err := r.client.SetNX(ctx, lockKey, "locked", defaultLockTimeout).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !ok {
		// Проверяем, не истек ли таймаут блокировки
		ttl, err := r.client.TTL(ctx, lockKey).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to check lock TTL: %w", err)
		}

		if ttl <= 0 {
			// Блокировка истекла, удаляем ее и пытаемся получить снова
			r.client.Del(ctx, lockKey)
			ok, err = r.client.SetNX(ctx, lockKey, "locked", defaultLockTimeout).Result()
			if err != nil {
				return nil, fmt.Errorf("failed to acquire lock after cleanup: %w", err)
			}
			if !ok {
				return nil, fmt.Errorf("failed to acquire lock: already locked")
			}
		} else {
			return nil, fmt.Errorf("failed to acquire lock: already locked (TTL: %v)", ttl)
		}
	}

	// Гарантируем освобождение блокировки
	defer func() {
		if err := r.client.Del(ctx, lockKey).Err(); err != nil {
			log.Printf("Failed to release lock for giveaway %s: %v", id, err)
		}
	}()

	giveaway, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return giveaway, nil
}

func (r *redisRepository) Update(ctx context.Context, giveaway *models.Giveaway) error {
	// Создаем копию гивевея без требований для основного хранения
	giveawayData := *giveaway
	giveawayData.Requirements = nil

	data, err := json.Marshal(giveawayData)
	if err != nil {
		return fmt.Errorf("failed to marshal giveaway: %w", err)
	}

	pipe := r.client.Pipeline()

	// Обновляем основные данные гивевея
	pipe.Set(ctx, makeGiveawayKey(giveaway.ID), data, 0)

	// Обновляем требования
	if len(giveaway.Requirements) > 0 {
		reqData, err := json.Marshal(giveaway.Requirements)
		if err != nil {
			return fmt.Errorf("failed to marshal requirements: %w", err)
		}
		pipe.Set(ctx, makeRequirementsKey(giveaway.ID), reqData, 0)
	} else {
		// Если требований нет, удаляем ключ
		pipe.Del(ctx, makeRequirementsKey(giveaway.ID))
	}

	_, err = pipe.Exec(ctx)
	return err
}

func (r *redisRepository) UpdateTx(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway) error {
	data, err := json.Marshal(giveaway)
	if err != nil {
		return err
	}
	redisTx := tx.(*redisTransaction)
	redisTx.pipe.Set(ctx, makeGiveawayKey(giveaway.ID), data, 0)
	return nil
}

func (r *redisRepository) Delete(ctx context.Context, id string) error {
	pipe := r.client.Pipeline()
	pipe.Del(ctx, makeGiveawayKey(id))
	pipe.Del(ctx, makeRequirementsKey(id))
	pipe.Del(ctx, makeParticipantsCountKey(id))
	pipe.SRem(ctx, keyActiveGiveaways, id)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisRepository) GetActiveGiveaways(ctx context.Context) ([]string, error) {
	return r.client.SMembers(ctx, keyActiveGiveaways).Result()
}

func (r *redisRepository) AddToPending(ctx context.Context, id string) error {
	pipe := r.client.Pipeline()
	pipe.SRem(ctx, keyActiveGiveaways, id)
	pipe.SAdd(ctx, keyPendingGiveaways, id)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisRepository) AddToPendingTx(ctx context.Context, tx repository.Transaction, id string) error {
	redisTx := tx.(*redisTransaction)
	redisTx.pipe.SRem(ctx, keyActiveGiveaways, id)
	redisTx.pipe.SAdd(ctx, keyPendingGiveaways, id)
	return nil
}

func (r *redisRepository) AddToHistory(ctx context.Context, id string) error {
	pipe := r.client.Pipeline()
	pipe.SRem(ctx, keyPendingGiveaways, id)
	pipe.SAdd(ctx, keyHistoryGiveaways, id)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisRepository) AddToHistoryTx(ctx context.Context, tx repository.Transaction, id string) error {
	redisTx := tx.(*redisTransaction)
	redisTx.pipe.SRem(ctx, keyPendingGiveaways, id)
	redisTx.pipe.SAdd(ctx, keyHistoryGiveaways, id)
	return nil
}

func (r *redisRepository) AddParticipant(ctx context.Context, giveawayID string, userID int64) error {
	pipe := r.client.Pipeline()
	pipe.SAdd(ctx, makeGiveawayKey(giveawayID)+":participants", userID)
	pipe.Incr(ctx, makeParticipantsCountKey(giveawayID))
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisRepository) GetParticipants(ctx context.Context, giveawayID string) ([]int64, error) {
	members, err := r.client.SMembers(ctx, makeGiveawayKey(giveawayID)+":participants").Result()
	if err != nil {
		return nil, err
	}

	participants := make([]int64, len(members))
	for i, member := range members {
		participants[i], err = strconv.ParseInt(member, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	return participants, nil
}

func (r *redisRepository) GetParticipantsCount(ctx context.Context, giveawayID string) (int64, error) {
	count, err := r.client.Get(ctx, makeParticipantsCountKey(giveawayID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return count, err
}

func (r *redisRepository) IsParticipant(ctx context.Context, giveawayID string, userID int64) (bool, error) {
	return r.client.SIsMember(ctx, makeGiveawayKey(giveawayID)+":participants", userID).Result()
}

func (r *redisRepository) SelectWinners(ctx context.Context, giveawayID string, count int) ([]models.Winner, error) {
	participants, err := r.GetParticipants(ctx, giveawayID)
	if err != nil {
		return nil, err
	}

	if len(participants) < count {
		count = len(participants)
	}

	winners := make([]models.Winner, count)
	for i := 0; i < count; i++ {
		idx := rand.Intn(len(participants))
		userID := participants[idx]

		user, err := r.GetUser(ctx, userID)
		if err != nil {
			return nil, err
		}

		winners[i] = models.Winner{
			UserID:   userID,
			Username: user.Username,
			Place:    i + 1,
		}

		participants = append(participants[:idx], participants[idx+1:]...)
	}

	return winners, nil
}

func (r *redisRepository) SelectWinnersTx(ctx context.Context, tx repository.Transaction, giveawayID string, count int) ([]models.Winner, error) {
	winners, err := r.SelectWinners(ctx, giveawayID, count)
	if err != nil {
		return nil, err
	}

	redisTx := tx.(*redisTransaction)
	winnersData, err := json.Marshal(winners)
	if err != nil {
		return nil, err
	}
	redisTx.pipe.Set(ctx, makeGiveawayKey(giveawayID)+":winners", winnersData, 0)

	return winners, nil
}

func (r *redisRepository) GetWinners(ctx context.Context, giveawayID string) ([]models.Winner, error) {
	data, err := r.client.Get(ctx, makeGiveawayKey(giveawayID)+":winners").Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var winners []models.Winner
	if err := json.Unmarshal(data, &winners); err != nil {
		return nil, err
	}

	return winners, nil
}

func (r *redisRepository) CreatePrize(ctx context.Context, prize *models.Prize) error {
	data, err := json.Marshal(prize)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, makePrizeKey(prize.ID), data, 0).Err()
}

func (r *redisRepository) GetPrize(ctx context.Context, id string) (*models.Prize, error) {
	data, err := r.client.Get(ctx, makePrizeKey(id)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("prize not found")
	}
	if err != nil {
		return nil, err
	}

	var prize models.Prize
	if err := json.Unmarshal(data, &prize); err != nil {
		return nil, err
	}

	return &prize, nil
}

func (r *redisRepository) GetPrizeTx(ctx context.Context, tx repository.Transaction, id string) (*models.Prize, error) {
	return r.GetPrize(ctx, id)
}

func (r *redisRepository) GetPrizes(ctx context.Context, giveawayID string) ([]models.PrizePlace, error) {
	giveaway, err := r.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, err
	}
	return giveaway.Prizes, nil
}

func (r *redisRepository) GetPrizesTx(ctx context.Context, tx repository.Transaction, giveawayID string) ([]models.PrizePlace, error) {
	return r.GetPrizes(ctx, giveawayID)
}

func (r *redisRepository) AssignPrizeTx(ctx context.Context, tx repository.Transaction, userID int64, prizeID string, place int) error {
	redisTx := tx.(*redisTransaction)
	data := struct {
		UserID int64
		Place  int
	}{
		UserID: userID,
		Place:  place,
	}
	assignmentData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	redisTx.pipe.Set(ctx, makePrizeKey(prizeID)+":assignment", assignmentData, 0)
	return nil
}

func (r *redisRepository) GetPrizeTemplates(ctx context.Context) ([]*models.PrizeTemplate, error) {
	data, err := r.client.Get(ctx, keyPrizeTemplates).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var templates []*models.PrizeTemplate
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, err
	}

	return templates, nil
}

func (r *redisRepository) AddTickets(ctx context.Context, giveawayID string, userID int64, count int64) error {
	return r.client.HIncrBy(ctx, makeGiveawayKey(giveawayID)+":tickets", strconv.FormatInt(userID, 10), count).Err()
}

func (r *redisRepository) GetUserTickets(ctx context.Context, giveawayID string, userID int64) (int, error) {
	tickets, err := r.client.HGet(ctx, makeGiveawayKey(giveawayID)+":tickets", strconv.FormatInt(userID, 10)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return tickets, err
}

func (r *redisRepository) GetTotalTickets(ctx context.Context, giveawayID string) (int, error) {
	tickets, err := r.client.HVals(ctx, makeGiveawayKey(giveawayID)+":tickets").Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	total := 0
	for _, ticket := range tickets {
		count, err := strconv.Atoi(ticket)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (r *redisRepository) RemoveFromActive(ctx context.Context, id string) error {
	return r.client.SRem(ctx, keyActiveGiveaways, id).Err()
}

func (r *redisRepository) DeleteParticipantsCount(ctx context.Context, id string) error {
	return r.client.Del(ctx, makeParticipantsCountKey(id)).Err()
}

func (r *redisRepository) DeletePrizes(ctx context.Context, id string) error {
	pattern := keyPrefixPrize + id + ":*"
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return r.client.Del(ctx, keys...).Err()
	}
	return nil
}

func (r *redisRepository) GetByCreatorAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error) {
	var result []*models.Giveaway

	// Получаем все гивевеи из всех множеств
	allIds := make(map[string]bool)

	// Собираем ID из всех множеств
	activeIds, err := r.client.SMembers(ctx, keyActiveGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active giveaway ids: %w", err)
	}
	for _, id := range activeIds {
		allIds[id] = true
	}

	pendingIds, err := r.client.SMembers(ctx, keyPendingGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending giveaway ids: %w", err)
	}
	for _, id := range pendingIds {
		allIds[id] = true
	}

	historyIds, err := r.client.SMembers(ctx, keyHistoryGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get history giveaway ids: %w", err)
	}
	for _, id := range historyIds {
		allIds[id] = true
	}

	// Проверяем каждый гивевей
	for id := range allIds {
		giveaway, err := r.GetByID(ctx, id)
		if err != nil {
			if err == repository.ErrGiveawayNotFound {
				continue
			}
			return nil, fmt.Errorf("failed to get giveaway %s: %w", id, err)
		}

		// Проверяем принадлежность пользователю и статус
		if giveaway.CreatorID == userID {
			for _, status := range statuses {
				if giveaway.Status == status {
					result = append(result, giveaway)
					break
				}
			}
		}
	}

	return result, nil
}

func (r *redisRepository) GetByParticipantAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error) {
	var result []*models.Giveaway

	// Получаем все гивевеи из всех множеств
	allIds := make(map[string]bool)

	// Собираем ID из всех множеств
	activeIds, err := r.client.SMembers(ctx, keyActiveGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active giveaway ids: %w", err)
	}
	for _, id := range activeIds {
		allIds[id] = true
	}

	pendingIds, err := r.client.SMembers(ctx, keyPendingGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending giveaway ids: %w", err)
	}
	for _, id := range pendingIds {
		allIds[id] = true
	}

	historyIds, err := r.client.SMembers(ctx, keyHistoryGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get history giveaway ids: %w", err)
	}
	for _, id := range historyIds {
		allIds[id] = true
	}

	// Проверяем каждый гивевей
	for id := range allIds {
		isParticipant, err := r.IsParticipant(ctx, id, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check participant status: %w", err)
		}

		if isParticipant {
			giveaway, err := r.GetByID(ctx, id)
			if err != nil {
				if err == repository.ErrGiveawayNotFound {
					continue
				}
				return nil, fmt.Errorf("failed to get giveaway %s: %w", id, err)
			}

			// Проверяем статус
			for _, status := range statuses {
				if giveaway.Status == status {
					result = append(result, giveaway)
					break
				}
			}
		}
	}

	return result, nil
}

func (r *redisRepository) MoveToHistory(ctx context.Context, id string) error {
	pipe := r.client.Pipeline()
	pipe.SRem(ctx, keyActiveGiveaways, id)
	pipe.SRem(ctx, keyPendingGiveaways, id)
	pipe.SAdd(ctx, keyHistoryGiveaways, id)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisRepository) GetCreator(ctx context.Context, userID int64) (*usermodels.User, error) {
	return r.GetUser(ctx, userID)
}

func (r *redisRepository) GetUser(ctx context.Context, userID int64) (*usermodels.User, error) {
	data, err := r.client.Get(ctx, fmt.Sprintf("user:%d", userID)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}

	var user usermodels.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *redisRepository) GetPendingGiveaways(ctx context.Context) ([]*models.Giveaway, error) {
	ids, err := r.client.SMembers(ctx, keyPendingGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending giveaway ids: %w", err)
	}

	giveaways := make([]*models.Giveaway, 0, len(ids))
	for _, id := range ids {
		giveaway, err := r.GetByID(ctx, id)
		if err != nil {
			if err == repository.ErrGiveawayNotFound {
				continue
			}
			return nil, fmt.Errorf("failed to get giveaway %s: %w", id, err)
		}

		// Проверяем статус и обновляем принадлежность к соответствующему множеству
		pipe := r.client.Pipeline()
		switch giveaway.Status {
		case models.GiveawayStatusActive:
			pipe.SRem(ctx, keyPendingGiveaways, id)
			pipe.SAdd(ctx, keyActiveGiveaways, id)
		case models.GiveawayStatusCompleted, models.GiveawayStatusHistory, models.GiveawayStatusCancelled:
			pipe.SRem(ctx, keyPendingGiveaways, id)
			pipe.SAdd(ctx, keyHistoryGiveaways, id)
		case models.GiveawayStatusPending, models.GiveawayStatusProcessing:
			giveaways = append(giveaways, giveaway)
		}
		pipe.Exec(ctx)
	}

	return giveaways, nil
}

// CleanupInconsistentData проверяет и очищает несогласованные данные
func (r *redisRepository) CleanupInconsistentData(ctx context.Context) error {
	// Получаем все ID из всех списков статусов
	activeIDs, err := r.client.SMembers(ctx, keyActiveGiveaways).Result()
	if err != nil {
		return fmt.Errorf("failed to get active giveaways: %w", err)
	}

	pendingIDs, err := r.client.SMembers(ctx, keyPendingGiveaways).Result()
	if err != nil {
		return fmt.Errorf("failed to get pending giveaways: %w", err)
	}

	historyIDs, err := r.client.SMembers(ctx, keyHistoryGiveaways).Result()
	if err != nil {
		return fmt.Errorf("failed to get history giveaways: %w", err)
	}

	// Проверяем каждый ID
	allIDs := make(map[string][]string)
	for _, id := range activeIDs {
		allIDs[id] = append(allIDs[id], "active")
	}
	for _, id := range pendingIDs {
		allIDs[id] = append(allIDs[id], "pending")
	}
	for _, id := range historyIDs {
		allIDs[id] = append(allIDs[id], "history")
	}

	for id, statuses := range allIDs {
		// Проверяем существование основных данных розыгрыша
		exists, err := r.client.Exists(ctx, makeGiveawayKey(id)).Result()
		if err != nil {
			return fmt.Errorf("failed to check giveaway existence: %w", err)
		}

		if exists == 0 {
			// Розыгрыш не существует, но есть в списках статусов
			pipe := r.client.Pipeline()
			pipe.SRem(ctx, keyActiveGiveaways, id)
			pipe.SRem(ctx, keyPendingGiveaways, id)
			pipe.SRem(ctx, keyHistoryGiveaways, id)
			pipe.Del(ctx, makeParticipantsCountKey(id))
			pipe.Del(ctx, makeGiveawayKey(id)+":participants")
			pipe.Del(ctx, makeGiveawayKey(id)+":tickets")
			pipe.Del(ctx, makeGiveawayKey(id)+":winners")

			// Удаляем все связанные с призами данные
			pattern := keyPrefixPrize + id + ":*"
			keys, err := r.client.Keys(ctx, pattern).Result()
			if err != nil {
				return fmt.Errorf("failed to get prize keys: %w", err)
			}
			if len(keys) > 0 {
				pipe.Del(ctx, keys...)
			}

			if _, err := pipe.Exec(ctx); err != nil {
				return fmt.Errorf("failed to cleanup orphaned data: %w", err)
			}
			continue
		}

		if len(statuses) > 1 {
			// Розыгрыш находится в нескольких списках статусов
			// Получаем актуальный статус из данных розыгрыша
			giveaway, err := r.GetByID(ctx, id)
			if err != nil {
				return fmt.Errorf("failed to get giveaway: %w", err)
			}

			// Очищаем все статусы и устанавливаем правильный
			pipe := r.client.Pipeline()
			pipe.SRem(ctx, keyActiveGiveaways, id)
			pipe.SRem(ctx, keyPendingGiveaways, id)
			pipe.SRem(ctx, keyHistoryGiveaways, id)

			switch giveaway.Status {
			case models.GiveawayStatusActive:
				pipe.SAdd(ctx, keyActiveGiveaways, id)
			case models.GiveawayStatusPending:
				pipe.SAdd(ctx, keyPendingGiveaways, id)
			case models.GiveawayStatusCompleted, models.GiveawayStatusHistory:
				pipe.SAdd(ctx, keyHistoryGiveaways, id)
			}

			if _, err := pipe.Exec(ctx); err != nil {
				return fmt.Errorf("failed to fix status inconsistency: %w", err)
			}
		}
	}

	return nil
}

// AcquireLock получает блокировку с таймаутом
func (r *redisRepository) AcquireLock(ctx context.Context, key string, timeout time.Duration) error {
	ok, err := r.client.SetNX(ctx, key, "locked", timeout).Result()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !ok {
		return repository.ErrAlreadyLocked
	}
	return nil
}

// ReleaseLock освобождает блокировку
func (r *redisRepository) ReleaseLock(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// CreateWinRecord создает запись о выигрыше
func (r *redisRepository) CreateWinRecord(ctx context.Context, record *models.WinRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal win record: %w", err)
	}
	return r.client.Set(ctx, fmt.Sprintf("win_record:%s", record.ID), data, 0).Err()
}

// CreateWinRecordTx создает запись о выигрыше в транзакции
func (r *redisRepository) CreateWinRecordTx(ctx context.Context, tx repository.Transaction, record *models.WinRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal win record: %w", err)
	}
	redisTx := tx.(*redisTransaction)
	redisTx.pipe.Set(ctx, fmt.Sprintf("win_record:%s", record.ID), data, 0)
	return nil
}

// GetWinRecord получает запись о выигрыше
func (r *redisRepository) GetWinRecord(ctx context.Context, id string) (*models.WinRecord, error) {
	data, err := r.client.Get(ctx, fmt.Sprintf("win_record:%s", id)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("win record not found")
	}
	if err != nil {
		return nil, err
	}

	var record models.WinRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// GetWinRecordsByGiveaway получает все записи о выигрышах для розыгрыша
func (r *redisRepository) GetWinRecordsByGiveaway(ctx context.Context, giveawayID string) ([]*models.WinRecord, error) {
	pattern := fmt.Sprintf("win_record:%s:*", giveawayID)
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	records := make([]*models.WinRecord, 0, len(keys))
	for _, key := range keys {
		data, err := r.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}

		var record models.WinRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}
		records = append(records, &record)
	}
	return records, nil
}

// UpdateWinRecord обновляет запись о выигрыше
func (r *redisRepository) UpdateWinRecord(ctx context.Context, record *models.WinRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal win record: %w", err)
	}
	return r.client.Set(ctx, fmt.Sprintf("win_record:%s", record.ID), data, 0).Err()
}

// UpdateWinRecordTx обновляет запись о выигрыше в транзакции
func (r *redisRepository) UpdateWinRecordTx(ctx context.Context, tx repository.Transaction, record *models.WinRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal win record: %w", err)
	}
	redisTx := tx.(*redisTransaction)
	redisTx.pipe.Set(ctx, fmt.Sprintf("win_record:%s", record.ID), data, 0)
	return nil
}

// DistributePrizeTx распределяет приз в транзакции
func (r *redisRepository) DistributePrizeTx(ctx context.Context, tx repository.Transaction, giveawayID string, userID int64, prizeID string) error {
	redisTx := tx.(*redisTransaction)
	key := fmt.Sprintf("prize:%s:distribution", prizeID)
	data := struct {
		GiveawayID string
		UserID     int64
		Time       time.Time
	}{
		GiveawayID: giveawayID,
		UserID:     userID,
		Time:       time.Now(),
	}
	distributionData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	redisTx.pipe.Set(ctx, key, distributionData, 0)
	return nil
}

// GetAllTicketsTx получает все билеты для розыгрыша в транзакции
func (r *redisRepository) GetAllTicketsTx(ctx context.Context, tx repository.Transaction, giveawayID string) (map[int64]int, error) {
	tickets := make(map[int64]int)
	data, err := r.client.HGetAll(ctx, makeGiveawayKey(giveawayID)+":tickets").Result()
	if err != nil {
		return nil, err
	}

	for userIDStr, countStr := range data {
		userID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			continue
		}
		count, err := strconv.Atoi(countStr)
		if err != nil {
			continue
		}
		tickets[userID] = count
	}

	return tickets, nil
}

// GetParticipantsTx получает список участников в транзакции
func (r *redisRepository) GetParticipantsTx(ctx context.Context, tx repository.Transaction, giveawayID string) ([]int64, error) {
	members, err := r.client.SMembers(ctx, makeGiveawayKey(giveawayID)+":participants").Result()
	if err != nil {
		return nil, err
	}

	participants := make([]int64, len(members))
	for i, member := range members {
		participants[i], err = strconv.ParseInt(member, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	return participants, nil
}

// AddToProcessingSet добавляет розыгрыш в множество обрабатываемых
func (r *redisRepository) AddToProcessingSet(ctx context.Context, id string) bool {
	return r.client.SAdd(ctx, keyProcessingSet, id).Val() > 0
}

// RemoveFromProcessingSet удаляет розыгрыш из множества обрабатываемых
func (r *redisRepository) RemoveFromProcessingSet(ctx context.Context, id string) error {
	return r.client.SRem(ctx, keyProcessingSet, id).Err()
}

// GetExpiredGiveaways возвращает список истекших розыгрышей
func (r *redisRepository) GetExpiredGiveaways(ctx context.Context, now int64) ([]string, error) {
	ids, err := r.client.SMembers(ctx, keyActiveGiveaways).Result()
	if err != nil {
		return nil, err
	}

	var expired []string
	for _, id := range ids {
		giveaway, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}
		if giveaway.StartedAt.Unix()+giveaway.Duration <= now {
			expired = append(expired, id)
		}
	}
	return expired, nil
}

// UpdateStatusAtomic атомарно обновляет статус розыгрыша
func (r *redisRepository) UpdateStatusAtomic(ctx context.Context, tx repository.Transaction, id string, update models.GiveawayStatusUpdate) error {
	giveaway, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if giveaway.Status != update.OldStatus {
		return fmt.Errorf("status mismatch: expected %s, got %s", update.OldStatus, giveaway.Status)
	}

	giveaway.Status = update.NewStatus
	giveaway.UpdatedAt = update.UpdatedAt

	redisTx := tx.(*redisTransaction)

	// Обновляем статус в основных данных
	data, err := json.Marshal(giveaway)
	if err != nil {
		return fmt.Errorf("failed to marshal giveaway: %w", err)
	}
	redisTx.pipe.Set(ctx, makeGiveawayKey(id), data, 0)

	// Обновляем принадлежность к множествам статусов
	switch update.NewStatus {
	case models.GiveawayStatusActive:
		redisTx.pipe.SAdd(ctx, keyActiveGiveaways, id)
		redisTx.pipe.SRem(ctx, keyPendingGiveaways, id)
		redisTx.pipe.SRem(ctx, keyHistoryGiveaways, id)
	case models.GiveawayStatusPending:
		redisTx.pipe.SRem(ctx, keyActiveGiveaways, id)
		redisTx.pipe.SAdd(ctx, keyPendingGiveaways, id)
		redisTx.pipe.SRem(ctx, keyHistoryGiveaways, id)
	case models.GiveawayStatusCompleted:
		redisTx.pipe.SRem(ctx, keyActiveGiveaways, id)
		redisTx.pipe.SRem(ctx, keyPendingGiveaways, id)
		redisTx.pipe.SAdd(ctx, keyHistoryGiveaways, id)
	case models.GiveawayStatusHistory:
		redisTx.pipe.SRem(ctx, keyActiveGiveaways, id)
		redisTx.pipe.SRem(ctx, keyPendingGiveaways, id)
		redisTx.pipe.SAdd(ctx, keyHistoryGiveaways, id)
	case models.GiveawayStatusCancelled:
		redisTx.pipe.SRem(ctx, keyActiveGiveaways, id)
		redisTx.pipe.SRem(ctx, keyPendingGiveaways, id)
		redisTx.pipe.SAdd(ctx, keyHistoryGiveaways, id)
	}

	return nil
}

// GetTopGiveaways returns top giveaways by participants count, then by soonest end time
func (r *redisRepository) GetTopGiveaways(ctx context.Context, limit int) ([]*models.Giveaway, error) {
	// Get all active and pending giveaways
	activeIDs, err := r.client.SMembers(ctx, keyActiveGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active giveaways: %w", err)
	}

	pendingIDs, err := r.client.SMembers(ctx, keyPendingGiveaways).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending giveaways: %w", err)
	}

	// Combine IDs
	allIDs := append(activeIDs, pendingIDs...)

	// Собираем данные по каждому гиву
	type giveawayScore struct {
		id     string
		score  int64
		endsAt int64 // unix timestamp
	}
	scores := make([]giveawayScore, 0, len(allIDs))
	for _, id := range allIDs {
		count, err := r.GetParticipantsCount(ctx, id)
		if err != nil {
			continue
		}
		giveaway, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}
		endsAt := giveaway.StartedAt.Unix() + giveaway.Duration
		scores = append(scores, giveawayScore{id: id, score: count, endsAt: endsAt})
	}

	// Сортируем: сначала по количеству участников (desc), потом по endsAt (asc)
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].endsAt < scores[j].endsAt
		}
		return scores[i].score > scores[j].score
	})

	// Get top N giveaways
	if limit > len(scores) {
		limit = len(scores)
	}
	scores = scores[:limit]

	// Get giveaway details
	result := make([]*models.Giveaway, 0, limit)
	for _, score := range scores {
		giveaway, err := r.GetByID(ctx, score.id)
		if err != nil {
			continue
		}
		result = append(result, giveaway)
	}

	return result, nil
}

// GetRequirementTemplates возвращает список доступных шаблонов требований из Redis
func (r *redisRepository) GetRequirementTemplates(ctx context.Context) ([]*models.RequirementTemplate, error) {
	// Проверяем, есть ли шаблоны в Redis
	exists, err := r.client.Exists(ctx, keyRequirementTemplates).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to check if requirement templates exist: %w", err)
	}

	// Если нет, создаем дефолтные шаблоны
	if exists == 0 {
		// Дефолтные шаблоны
		defaultTemplates := []*models.RequirementTemplate{
			{
				ID:   "channel_subscription_en",
				Name: "Channel Subscription",
				Type: "subscription",
			},
		}

		// Сохраняем шаблоны в Redis
		pipe := r.client.Pipeline()
		for _, template := range defaultTemplates {
			data, err := json.Marshal(template)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal requirement template: %w", err)
			}
			pipe.SAdd(ctx, keyRequirementTemplates, data)
		}
		_, err := pipe.Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to save default requirement templates: %w", err)
		}
	}

	// Получаем шаблоны из Redis
	templateData, err := r.client.SMembers(ctx, keyRequirementTemplates).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get requirement templates: %w", err)
	}

	// Преобразуем JSON в структуры
	templates := make([]*models.RequirementTemplate, 0, len(templateData))
	for _, data := range templateData {
		var template models.RequirementTemplate
		if err := json.Unmarshal([]byte(data), &template); err != nil {
			return nil, fmt.Errorf("failed to unmarshal requirement template: %w", err)
		}
		templates = append(templates, &template)
	}

	return templates, nil
}

// UpdateStatus обновляет статус розыгрыша
func (r *redisRepository) UpdateStatus(ctx context.Context, id string, status models.GiveawayStatus) error {
	giveaway, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}

	giveaway.Status = status
	giveaway.UpdatedAt = time.Now()

	return r.Update(ctx, giveaway)
}

// UpdateStatusIfPending атомарно обновляет статус розыгрыша, только если он находится в статусе pending
func (r *redisRepository) UpdateStatusIfPending(ctx context.Context, id string, status models.GiveawayStatus) (bool, error) {
	// Начинаем транзакцию
	tx, err := r.BeginTx(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Получаем розыгрыш с блокировкой
	giveaway, err := r.GetByIDWithLock(ctx, tx, id)
	if err != nil {
		return false, err
	}

	// Проверяем статус
	if giveaway.Status != models.GiveawayStatusPending {
		return false, nil
	}

	// Обновляем статус
	giveaway.Status = status
	giveaway.UpdatedAt = time.Now()

	// Сохраняем изменения
	if err := r.UpdateTx(ctx, tx, giveaway); err != nil {
		return false, err
	}

	// Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return true, nil
}

func (r *redisRepository) CancelGiveaway(ctx context.Context, giveawayID string) error {
	// Получаем гивевей
	giveaway, err := r.GetByID(ctx, giveawayID)
	if err != nil {
		return err
	}

	// Проверяем условия для отмены
	participantsCount, err := r.GetParticipantsCount(ctx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get participants count: %w", err)
	}

	if participantsCount > models.MaxParticipantsForCancel {
		return fmt.Errorf("cannot cancel giveaway with more than %d participants", models.MaxParticipantsForCancel)
	}

	timeSinceCreation := time.Since(giveaway.CreatedAt)
	if timeSinceCreation.Minutes() > float64(models.MaxCancellationTimeMinutes) {
		return fmt.Errorf("cannot cancel giveaway older than %d minutes", models.MaxCancellationTimeMinutes)
	}

	// Начинаем транзакцию
	tx := r.client.TxPipeline()

	// Обновляем статус
	giveaway.Status = models.GiveawayStatusCancelled
	giveaway.UpdatedAt = time.Now()

	data, err := json.Marshal(giveaway)
	if err != nil {
		return err
	}

	// Обновляем основные данные
	tx.Set(ctx, makeGiveawayKey(giveawayID), data, 0)

	// Удаляем из множества активных
	tx.SRem(ctx, keyActiveGiveaways, giveawayID)

	// Добавляем в множество исторических
	tx.SAdd(ctx, keyHistoryGiveaways, giveawayID)

	// Выполняем транзакцию
	_, err = tx.Exec(ctx)
	return err
}

// SetChannelAvatar сохраняет аватар канала в Redis на 15 минут
func (r *redisRepository) SetChannelAvatar(ctx context.Context, channelID string, avatarURL string) error {
	key := channelAvatarKey + channelID
	return r.client.Set(ctx, key, avatarURL, 15*time.Minute).Err()
}

// GetChannelAvatar получает аватар канала из Redis
func (r *redisRepository) GetChannelAvatar(ctx context.Context, channelID string) (string, error) {
	return r.client.Get(ctx, channelAvatarKey+channelID).Result()
}

// GetChannelUsername получает username канала из Redis
func (r *redisRepository) GetChannelUsername(ctx context.Context, channelID int64) (string, error) {
	key := fmt.Sprintf("channel:%d:username", channelID)
	return r.client.Get(ctx, key).Result()
}
