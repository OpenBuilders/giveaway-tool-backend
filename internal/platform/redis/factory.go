package redis

import (
	"context"
	"fmt"
	"giveaway-tool-backend/internal/common/config"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient interface {
	Ping(ctx context.Context) *redis.StatusCmd
	Set(ctx context.Context, key string, value interface{}, ttl ...time.Duration) *redis.StatusCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
	SIsMember(ctx context.Context, key string, member interface{}) *redis.BoolCmd
	Incr(ctx context.Context, key string) *redis.IntCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HGet(ctx context.Context, key, field string) *redis.StringCmd
	HIncrBy(ctx context.Context, key, field string, incr int64) *redis.IntCmd
	HVals(ctx context.Context, key string) *redis.StringSliceCmd
	Keys(ctx context.Context, pattern string) *redis.StringSliceCmd
	Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd
	Pipeline() redis.Pipeliner
	ConfigSet(ctx context.Context, parameter, value string) *redis.StatusCmd
	Close() error
}

func CreateRedisClient(cfg *config.Config) (RedisClient, error) {
	if !cfg.Redis.EnableSharding {

		client := redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})

		return &redisClientWrapper{client: client}, nil
	}

	return createShardedRedisClientFromConfig(cfg)
}

func NewShardedRedisClientFromConfig(cfg *config.Config) (RedisClient, error) {
	if !cfg.Redis.EnableSharding {
		client := redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})

		return &redisClientWrapper{client: client}, nil
	}

	return createShardedRedisClientFromConfig(cfg)
}

func createShardedRedisClientFromConfig(cfg *config.Config) (*ShardedRedisClient, error) {
	writeConfigs, err := parseShardConfigs(cfg.Redis.WriteShards)
	if err != nil {
		return nil, fmt.Errorf("failed to parse write shards config: %w", err)
	}

	readConfigs, err := parseShardConfigs(cfg.Redis.ReadShards)
	if err != nil {
		return nil, fmt.Errorf("failed to parse read shards config: %w", err)
	}

	if len(readConfigs) == 0 {
		readConfigs = writeConfigs
	}

	return NewShardedRedisClient(writeConfigs, readConfigs)
}

type redisClientWrapper struct {
	client *redis.Client
}

func (w *redisClientWrapper) Ping(ctx context.Context) *redis.StatusCmd {
	return w.client.Ping(ctx)
}

func (w *redisClientWrapper) Set(ctx context.Context, key string, value interface{}, ttl ...time.Duration) *redis.StatusCmd {
	if len(ttl) > 0 {
		return w.client.Set(ctx, key, value, ttl[0])
	}
	return w.client.Set(ctx, key, value, 0)
}

func (w *redisClientWrapper) Get(ctx context.Context, key string) *redis.StringCmd {
	return w.client.Get(ctx, key)
}

func (w *redisClientWrapper) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return w.client.Del(ctx, keys...)
}

func (w *redisClientWrapper) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	return w.client.Exists(ctx, keys...)
}

func (w *redisClientWrapper) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	return w.client.SAdd(ctx, key, members...)
}

func (w *redisClientWrapper) SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	return w.client.SRem(ctx, key, members...)
}

func (w *redisClientWrapper) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	return w.client.SMembers(ctx, key)
}

func (w *redisClientWrapper) SIsMember(ctx context.Context, key string, member interface{}) *redis.BoolCmd {
	return w.client.SIsMember(ctx, key, member)
}

func (w *redisClientWrapper) Incr(ctx context.Context, key string) *redis.IntCmd {
	return w.client.Incr(ctx, key)
}

func (w *redisClientWrapper) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	return w.client.HSet(ctx, key, values...)
}

func (w *redisClientWrapper) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	return w.client.HGet(ctx, key, field)
}

func (w *redisClientWrapper) HIncrBy(ctx context.Context, key, field string, incr int64) *redis.IntCmd {
	return w.client.HIncrBy(ctx, key, field, incr)
}

func (w *redisClientWrapper) HVals(ctx context.Context, key string) *redis.StringSliceCmd {
	return w.client.HVals(ctx, key)
}

func (w *redisClientWrapper) Keys(ctx context.Context, pattern string) *redis.StringSliceCmd {
	return w.client.Keys(ctx, pattern)
}

func (w *redisClientWrapper) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	return w.client.Scan(ctx, cursor, match, count)
}

func (w *redisClientWrapper) Pipeline() redis.Pipeliner {
	return w.client.Pipeline()
}

func (w *redisClientWrapper) ConfigSet(ctx context.Context, parameter, value string) *redis.StatusCmd {
	return w.client.ConfigSet(ctx, parameter, value)
}

func (w *redisClientWrapper) Close() error {
	return w.client.Close()
}

func parseShardConfigs(shardStrings []string) ([]ShardConfig, error) {
	if len(shardStrings) == 0 {
		return nil, fmt.Errorf("no shard configurations provided")
	}

	configs := make([]ShardConfig, 0, len(shardStrings))

	for i, shardStr := range shardStrings {
		parts := strings.Split(strings.TrimSpace(shardStr), ":")

		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid shard config format at index %d: %s (expected host:port[:password][:db])", i, shardStr)
		}

		config := ShardConfig{
			Host:   parts[0],
			Weight: 1,
		}

		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid port in shard config at index %d: %s", i, shardStr)
		}
		config.Port = port

		if len(parts) > 2 {
			config.Password = parts[2]
		}

		if len(parts) > 3 {
			db, err := strconv.Atoi(parts[3])
			if err != nil {
				return nil, fmt.Errorf("invalid database number in shard config at index %d: %s", i, shardStr)
			}
			config.DB = db
		}

		configs = append(configs, config)
	}

	return configs, nil
}

func GetShardStats(client RedisClient) map[string]interface{} {
	if shardedClient, ok := client.(*ShardedRedisClient); ok {
		return shardedClient.GetShardStats()
	}

	if wrapper, ok := client.(*redisClientWrapper); ok {
		return map[string]interface{}{
			"type": "single",
			"addr": wrapper.client.Options().Addr,
		}
	}

	return map[string]interface{}{
		"type": "unknown",
	}
}
