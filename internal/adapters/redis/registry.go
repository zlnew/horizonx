package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Registry struct {
	redis *redis.Client
}

func NewRegistry(r *redis.Client) *Registry {
	return &Registry{redis: r}
}

func (r *Registry) Append(ctx context.Context, stream string, payload any, maxLen int64) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("registry marshal failed: %w", err)
	}

	id, err := r.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{
			"data": data,
		},
		MaxLen: maxLen,
	}).Result()
	if err != nil {
		return "", fmt.Errorf("registry xadd failed: %w", err)
	}

	return id, nil
}

func (r *Registry) GetRangeAsc(ctx context.Context, stream string, limit int64) ([]redis.XMessage, error) {
	msgs, err := r.redis.XRangeN(ctx, stream, "-", "+", limit).Result()
	if err != nil {
		return nil, fmt.Errorf("registry xrange failed: %w", err)
	}

	return msgs, nil
}

func (r *Registry) GetRangeDesc(ctx context.Context, stream string, limit int64) ([]redis.XMessage, error) {
	msgs, err := r.redis.XRevRangeN(ctx, stream, "+", "-", limit).Result()
	if err != nil {
		return nil, fmt.Errorf("registry xrevrange failed: %w", err)
	}
	return msgs, nil
}

func (r *Registry) GetLatest(ctx context.Context, stream string) ([]redis.XMessage, error) {
	msgs, err := r.redis.XRevRangeN(ctx, stream, "+", "-", 1).Result()
	if err != nil {
		return nil, fmt.Errorf("registry xrevrange failed: %w", err)
	}

	return msgs, nil
}

func (r *Registry) Ack(ctx context.Context, stream string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	if err := r.redis.XDel(ctx, stream, ids...).Err(); err != nil {
		return fmt.Errorf("registry xdel failed: %w", err)
	}

	return nil
}
