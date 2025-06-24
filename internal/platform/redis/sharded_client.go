package redis

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type ShardConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	Weight   int    `json:"weight"`
}

type ShardedRedisClient struct {
	writeShards []*redis.Client
	readShards  []*redis.Client
	shardCount  int
	mu          sync.RWMutex
}

func NewShardedRedisClient(writeConfigs, readConfigs []ShardConfig) (*ShardedRedisClient, error) {
	if len(writeConfigs) == 0 {
		return nil, fmt.Errorf("at least one write shard is required")
	}

	client := &ShardedRedisClient{
		writeShards: make([]*redis.Client, len(writeConfigs)),
		readShards:  make([]*redis.Client, len(readConfigs)),
		shardCount:  len(writeConfigs),
	}

	for i, config := range writeConfigs {
		client.writeShards[i] = redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
			Password: config.Password,
			DB:       config.DB,
		})

		if err := client.writeShards[i].Ping(context.Background()).Err(); err != nil {
			return nil, fmt.Errorf("failed to connect to write shard %d: %w", i, err)
		}
	}

	for i, config := range readConfigs {
		client.readShards[i] = redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
			Password: config.Password,
			DB:       config.DB,
		})

		if err := client.readShards[i].Ping(context.Background()).Err(); err != nil {
			return nil, fmt.Errorf("failed to connect to read shard %d: %w", i, err)
		}
	}

	return client, nil
}

func (c *ShardedRedisClient) getShardIndex(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := h.Sum32()
	return int(hash % uint32(c.shardCount))
}

func (c *ShardedRedisClient) getWriteShard(key string) *redis.Client {
	index := c.getShardIndex(key)
	return c.writeShards[index]
}

func (c *ShardedRedisClient) getReadShard(key string) *redis.Client {
	if len(c.readShards) == 0 {
		return c.getWriteShard(key)
	}

	index := c.getShardIndex(key) % len(c.readShards)
	return c.readShards[index]
}

func (c *ShardedRedisClient) Set(ctx context.Context, key string, value interface{}, ttl ...time.Duration) *redis.StatusCmd {
	shard := c.getWriteShard(key)
	if len(ttl) > 0 {
		return shard.Set(ctx, key, value, ttl[0])
	}
	return shard.Set(ctx, key, value, 0)
}

func (c *ShardedRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	shard := c.getReadShard(key)
	return shard.Get(ctx, key)
}

func (c *ShardedRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	if len(keys) == 0 {
		return redis.NewIntCmd(ctx, "del")
	}

	shardKeys := make(map[*redis.Client][]string)
	for _, key := range keys {
		shard := c.getWriteShard(key)
		shardKeys[shard] = append(shardKeys[shard], key)
	}

	var totalDeleted int64
	for shard, shardKeyList := range shardKeys {
		cmd := shard.Del(ctx, shardKeyList...)
		if cmd.Err() != nil {
			return cmd
		}
		totalDeleted += cmd.Val()
	}

	result := redis.NewIntCmd(ctx, "del")
	result.SetVal(totalDeleted)
	return result
}

func (c *ShardedRedisClient) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	if len(keys) == 0 {
		return redis.NewIntCmd(ctx, "exists")
	}

	shardKeys := make(map[*redis.Client][]string)
	for _, key := range keys {
		shard := c.getReadShard(key)
		shardKeys[shard] = append(shardKeys[shard], key)
	}

	var totalExists int64
	for shard, shardKeyList := range shardKeys {
		cmd := shard.Exists(ctx, shardKeyList...)
		if cmd.Err() != nil {
			return cmd
		}
		totalExists += cmd.Val()
	}

	result := redis.NewIntCmd(ctx, "exists")
	result.SetVal(totalExists)
	return result
}

func (c *ShardedRedisClient) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	shard := c.getWriteShard(key)
	return shard.SAdd(ctx, key, members...)
}

func (c *ShardedRedisClient) SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	shard := c.getWriteShard(key)
	return shard.SRem(ctx, key, members...)
}

func (c *ShardedRedisClient) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	shard := c.getReadShard(key)
	return shard.SMembers(ctx, key)
}

func (c *ShardedRedisClient) SIsMember(ctx context.Context, key string, member interface{}) *redis.BoolCmd {
	shard := c.getReadShard(key)
	return shard.SIsMember(ctx, key, member)
}

func (c *ShardedRedisClient) Incr(ctx context.Context, key string) *redis.IntCmd {
	shard := c.getWriteShard(key)
	return shard.Incr(ctx, key)
}

func (c *ShardedRedisClient) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	shard := c.getWriteShard(key)
	return shard.HSet(ctx, key, values...)
}

func (c *ShardedRedisClient) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	shard := c.getReadShard(key)
	return shard.HGet(ctx, key, field)
}

func (c *ShardedRedisClient) HIncrBy(ctx context.Context, key, field string, incr int64) *redis.IntCmd {
	shard := c.getWriteShard(key)
	return shard.HIncrBy(ctx, key, field, incr)
}

func (c *ShardedRedisClient) HVals(ctx context.Context, key string) *redis.StringSliceCmd {
	shard := c.getReadShard(key)
	return shard.HVals(ctx, key)
}

func (c *ShardedRedisClient) Keys(ctx context.Context, pattern string) *redis.StringSliceCmd {
	var allKeys []string
	shards := c.getAllShards()

	for _, shard := range shards {
		cmd := shard.Keys(ctx, pattern)
		if cmd.Err() != nil {
			return cmd
		}
		allKeys = append(allKeys, cmd.Val()...)
	}

	sort.Strings(allKeys)

	result := redis.NewStringSliceCmd(ctx, "keys", pattern)
	result.SetVal(allKeys)
	return result
}

func (c *ShardedRedisClient) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	shard := c.writeShards[0]
	return shard.Scan(ctx, cursor, match, count)
}

func (c *ShardedRedisClient) Pipeline() redis.Pipeliner {
	return c.writeShards[0].Pipeline()
}

func (c *ShardedRedisClient) ConfigSet(ctx context.Context, parameter, value string) *redis.StatusCmd {
	shards := c.getAllShards()

	for _, shard := range shards {
		cmd := shard.ConfigSet(ctx, parameter, value)
		if cmd.Err() != nil {
			return cmd
		}
	}

	result := redis.NewStatusCmd(ctx, "config", "set", parameter, value)
	result.SetVal("OK")
	return result
}

func (c *ShardedRedisClient) getAllShards() []*redis.Client {
	var shards []*redis.Client
	shards = append(shards, c.writeShards...)
	shards = append(shards, c.readShards...)
	return shards
}

func (c *ShardedRedisClient) Close() error {
	var lastErr error

	for _, shard := range c.writeShards {
		if err := shard.Close(); err != nil {
			lastErr = err
		}
	}

	for _, shard := range c.readShards {
		if err := shard.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func (c *ShardedRedisClient) GetShardStats() map[string]interface{} {
	stats := make(map[string]interface{})

	writeStats := make([]map[string]interface{}, len(c.writeShards))
	for i, shard := range c.writeShards {
		writeStats[i] = map[string]interface{}{
			"index": i,
			"addr":  shard.Options().Addr,
		}
	}
	stats["write_shards"] = writeStats

	readStats := make([]map[string]interface{}, len(c.readShards))
	for i, shard := range c.readShards {
		readStats[i] = map[string]interface{}{
			"index": i,
			"addr":  shard.Options().Addr,
		}
	}
	stats["read_shards"] = readStats

	stats["total_shards"] = c.shardCount
	stats["read_shards_count"] = len(c.readShards)

	return stats
}

func (c *ShardedRedisClient) Ping(ctx context.Context) *redis.StatusCmd {
	// Используем первый write shard для ping
	if len(c.writeShards) > 0 {
		return c.writeShards[0].Ping(ctx)
	}
	return redis.NewStatusCmd(ctx, "ping")
}
