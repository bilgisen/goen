package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilgisen/goen/internal/logger"
	"github.com/redis/go-redis/v9"
)

type RedisLimiter struct {
	client *redis.Client
	key    string
	rpm    int // requests per minute
	tpm    int // tokens per minute
}

// NewRedisLimiter creates a distributed limiter
func NewRedisLimiter(redisURL, redisKey string, rpm, tpm int) (*RedisLimiter, error) {
	if rpm <= 0 || tpm <= 0 {
		return nil, fmt.Errorf("rate limits must be positive values (rpm: %d, tpm: %d)", rpm, tpm)
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("cannot connect to redis: %w", err)
	}

	return &RedisLimiter{
		client: client,
		key:    redisKey,
		rpm:    rpm,
		tpm:    tpm,
	}, nil
}

// estimateTokens provides a rough estimation of token count for the given text.
// For mixed content (like Turkish and English), we use a conservative estimate.
// This is an approximation and actual token count may vary.
func estimateTokens(text string) int {
	// Rough estimation: ~4 chars per token for English, ~2 for Turkish
	// Using 3 as a conservative middle ground for mixed content
	return len(strings.TrimSpace(text)) / 3
}

// WaitIfNeeded blocks if rate limits are reached
func (r *RedisLimiter) WaitIfNeeded(ctx context.Context, usedTokens int) error {
	// Check Redis connection
	if err := r.client.Ping(ctx).Err(); err != nil {
		logger.Get().Error().
			Err(err).
			Msg("Redis connection check failed, rate limiting may be inaccurate")
		// Continue without rate limiting rather than failing the request
		return nil
	}

	now := time.Now().Unix()
	minuteKey := fmt.Sprintf("%s:%d", r.key, now/60)

	pipe := r.client.TxPipeline()
	reqCount := pipe.Incr(ctx, minuteKey+":req")
	tokCount := pipe.IncrBy(ctx, minuteKey+":tok", int64(usedTokens))
	pipe.Expire(ctx, minuteKey+":req", time.Minute)
	pipe.Expire(ctx, minuteKey+":tok", time.Minute)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis pipeline failed: %w", err)
	}

	req := int(reqCount.Val())
	tok := int(tokCount.Val())

	if req > r.rpm || tok > r.tpm {
		// Calculate time until next minute window
		sleep := time.Until(time.Unix((now/60+1)*60, 0))
		
		logger.Get().Info().
			Int("requests", req).
			Int("max_requests", r.rpm).
			Int("tokens_used", tok).
			Int("max_tokens", r.tpm).
			Dur("wait_time", sleep).
			Msg("Rate limit reached, waiting for next window")
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
			// Reset counters for the new window
			return nil
		}
	}

	return nil
}
