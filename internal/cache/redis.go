package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/bilgisen/goen/internal/config"
	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
	prefix string
}

func NewRedisClient(cfg *config.Config) (*RedisClient, error) {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{
		client: client,
		prefix: "news:",
	}, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

func (r *RedisClient) IsProcessed(ctx context.Context, hash string) (bool, error) {
	exists, err := r.client.Exists(ctx, r.prefix+hash).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists error: %w", err)
	}
	return exists > 0, nil
}

func (r *RedisClient) MarkProcessed(ctx context.Context, hash string, ttl time.Duration) error {
	return r.client.Set(ctx, r.prefix+hash, "1", ttl).Err()
}

func (r *RedisClient) ClearProcessed(ctx context.Context) error {
	iter := r.client.Scan(ctx, 0, r.prefix+"*", 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("error scanning keys: %w", err)
	}

	if len(keys) > 0 {
		if err := r.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("error deleting keys: %w", err)
		}
	}

	return nil
}
