package cache

import (
	"context"
	"fmt"
	"time"
)

// MockRedisClient provides a mock implementation for testing when Redis is not available
type MockRedisClient struct {
	data   map[string]string
	prefix string
}

func NewMockRedisClient(cfg *config.Config) (*MockRedisClient, error) {
	return &MockRedisClient{
		data:   make(map[string]string),
		prefix: "news:",
	}, nil
}

func (m *MockRedisClient) Close() error {
	return nil
}

func (m *MockRedisClient) IsProcessed(ctx context.Context, hash string) (bool, error) {
	key := m.prefix + hash
	_, exists := m.data[key]
	return exists, nil
}

func (m *MockRedisClient) MarkProcessed(ctx context.Context, hash string, ttl time.Duration) error {
	key := m.prefix + hash
	m.data[key] = "1"
	return nil
}

func (m *MockRedisClient) ClearProcessed(ctx context.Context) error {
	m.data = make(map[string]string)
	return nil
}
