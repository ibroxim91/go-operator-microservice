package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// IncrBy oshiradi va kalitga muddat qo'yadi (Tourvisor kunlik limiti uchun).
func (r *RedisCache) IncrBy(ctx context.Context, key string, n int64, ttl time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incr := pipe.IncrBy(ctx, key, n)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// GetRaw kalit qiymatini qaytaradi; kalit yo'q bo'lsa ok=false.
func (r *RedisCache) GetRaw(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (r *RedisCache) SetRaw(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}
