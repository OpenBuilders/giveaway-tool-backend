package redis

import (
	"context"
	"github.com/redis/go-redis/v9"
)

func New(addr, pass string, db int) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       db,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic("redis: " + err.Error())
	}
	return rdb
}
