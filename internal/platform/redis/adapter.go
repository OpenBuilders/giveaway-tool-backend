package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisAdapter struct {
	client RedisClient
}

func NewRedisAdapter(client RedisClient) *RedisAdapter {
	return &RedisAdapter{client: client}
}

func (a *RedisAdapter) GetUnderlyingClient() *redis.Client {
	if wrapper, ok := a.client.(*redisClientWrapper); ok {
		return wrapper.client
	}

	if sharded, ok := a.client.(*ShardedRedisClient); ok {
		return sharded.writeShards[0]
	}

	return nil
}

func (a *RedisAdapter) Ping(ctx context.Context) *redis.StatusCmd {
	return a.client.Ping(ctx)
}

func (a *RedisAdapter) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd {
	return a.client.Set(ctx, key, value, ttl)
}

func (a *RedisAdapter) Get(ctx context.Context, key string) *redis.StringCmd {
	return a.client.Get(ctx, key)
}

func (a *RedisAdapter) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return a.client.Del(ctx, keys...)
}

func (a *RedisAdapter) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	return a.client.Exists(ctx, keys...)
}

func (a *RedisAdapter) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	return a.client.SAdd(ctx, key, members...)
}

func (a *RedisAdapter) SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	return a.client.SRem(ctx, key, members...)
}

func (a *RedisAdapter) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	return a.client.SMembers(ctx, key)
}

func (a *RedisAdapter) SIsMember(ctx context.Context, key string, member interface{}) *redis.BoolCmd {
	return a.client.SIsMember(ctx, key, member)
}

func (a *RedisAdapter) Incr(ctx context.Context, key string) *redis.IntCmd {
	return a.client.Incr(ctx, key)
}

func (a *RedisAdapter) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	return a.client.HSet(ctx, key, values...)
}

func (a *RedisAdapter) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	return a.client.HGet(ctx, key, field)
}

func (a *RedisAdapter) HIncrBy(ctx context.Context, key, field string, incr int64) *redis.IntCmd {
	return a.client.HIncrBy(ctx, key, field, incr)
}

func (a *RedisAdapter) HVals(ctx context.Context, key string) *redis.StringSliceCmd {
	return a.client.HVals(ctx, key)
}

func (a *RedisAdapter) Keys(ctx context.Context, pattern string) *redis.StringSliceCmd {
	return a.client.Keys(ctx, pattern)
}

func (a *RedisAdapter) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	return a.client.Scan(ctx, cursor, match, count)
}

func (a *RedisAdapter) Pipeline() redis.Pipeliner {
	return a.client.Pipeline()
}

func (a *RedisAdapter) ConfigSet(ctx context.Context, parameter, value string) *redis.StatusCmd {
	return a.client.ConfigSet(ctx, parameter, value)
}

func (a *RedisAdapter) Close() error {
	return a.client.Close()
}
