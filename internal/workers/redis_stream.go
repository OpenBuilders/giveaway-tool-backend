package workers

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/open-builders/giveaway-backend/internal/platform/redis"
	"github.com/open-builders/giveaway-backend/internal/repository/postgres"
	go_redis "github.com/redis/go-redis/v9"
)

const streamKey = "bot:events"
const consumerGroup = "giveaway_backend_consumers"
const consumerName = "giveaway_worker_1"

type RedisStreamWorker struct {
	rdb  *redis.Client
	repo *postgres.GiveawayRepository
}

func NewRedisStreamWorker(rdb *redis.Client, repo *postgres.GiveawayRepository) *RedisStreamWorker {
	return &RedisStreamWorker{
		rdb:  rdb,
		repo: repo,
	}
}

// Start begins listening to the Redis stream for events.
func (w *RedisStreamWorker) Start(ctx context.Context) {
	// Ensure consumer group exists
	err := w.rdb.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		log.Printf("Error creating consumer group: %v", err)
	}

	log.Println("Starting Redis stream worker...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping Redis stream worker...")
			return
		default:
			// Read new entries from the stream
			entries, err := w.rdb.XReadGroup(ctx, &go_redis.XReadGroupArgs{
				Group:    consumerGroup,
				Consumer: consumerName,
				Streams:  []string{streamKey, ">"},
				Count:    1,
				Block:    5 * time.Second, // block for 5 seconds
			}).Result()

			if err != nil {
				if err.Error() != "redis: nil" { // timeout/no messages
					log.Printf("Error reading from stream: %v", err)
					time.Sleep(1 * time.Second) // backoff on error
				}
				continue
			}

			for _, stream := range entries {
				for _, msg := range stream.Messages {
					w.processMessage(ctx, msg.Values)
					// Acknowledge the message
					w.rdb.XAck(ctx, streamKey, consumerGroup, msg.ID)
				}
			}
		}
	}
}

func (w *RedisStreamWorker) processMessage(ctx context.Context, values map[string]interface{}) {
	eventType, ok := values["type"].(string)
	if !ok {
		return
	}

	if eventType == "bot_removed" {
		channelIDStr, ok := values["channel_id"].(string)
		if !ok {
			log.Printf("Invalid channel_id in bot_removed event: %v", values)
			return
		}

		channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
		if err != nil {
			log.Printf("Error parsing channel_id: %v", err)
			return
		}

		log.Printf("Processing bot_removed event for channel %d", channelID)

		if err := w.repo.RemoveRequirementsByChannelID(ctx, channelID); err != nil {
			log.Printf("Error removing requirements for channel %d: %v", channelID, err)
		} else {
			log.Printf("Successfully removed requirements for channel %d", channelID)
		}
	}
}
