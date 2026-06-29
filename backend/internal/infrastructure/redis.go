package infrastructure

import (
	"context"
	"fmt"

	"github.com/amll-dev/amll-hub/backend/internal/config"
	"github.com/redis/go-redis/v9"
)

// NewRedis 初始化 Redis 客户端
func NewRedis(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
