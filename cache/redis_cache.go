package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-operator-service/models"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache() (*RedisCache, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	password := os.Getenv("REDIS_PASSWORD")
	db := 0
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		parsed, err := strconv.Atoi(dbStr)
		if err == nil {
			db = parsed
		}
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	return &RedisCache{client: client}, nil
}

func (r *RedisCache) GetCachedResponse(ctx context.Context, key string) (*models.ResultResponse, error) {
	value, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var resp models.ResultResponse
	if err := json.Unmarshal([]byte(value), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (r *RedisCache) SetCachedResponse(ctx context.Context, key string, resp *models.ResultResponse, ttl time.Duration) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, payload, ttl).Err()
}

func (r *RedisCache) GetCachedAsyncResult(ctx context.Context, key string) (*models.AsyncSamoResult, error) {
	value, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var resp models.AsyncSamoResult
	if err := json.Unmarshal([]byte(value), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (r *RedisCache) SetCachedAsyncResult(ctx context.Context, key string, resp *models.AsyncSamoResult, ttl time.Duration) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, payload, ttl).Err()
}

func (r *RedisCache) GetCachedStreamResult(
    ctx context.Context,
    key string,
) (*models.StreamCacheResult, error) {

    value, err := r.client.Get(ctx, key).Result()

    if err != nil {

        if err == redis.Nil {
            return nil, nil
        }

        return nil, err
    }

    var result models.StreamCacheResult

    if err := json.Unmarshal(
        []byte(value),
        &result,
    ); err != nil {
        return nil, err
    }

    return &result, nil
}

func (r *RedisCache) GetOrSetCachedResponse(ctx context.Context, key string, ttl time.Duration, fetch func() (*models.ResultResponse, error)) (*models.ResultResponse, error) {
	cached, err := r.GetCachedResponse(ctx, key)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		return cached, nil
	}

	resp, err := fetch()
	if err != nil {
		return nil, err
	}
	if resp != nil && len(resp.Prices) > 0 {
		if err := r.SetCachedResponse(ctx, key, resp, ttl); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func GenerateCacheKey(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key == "PRICEPAGE" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(params[key])
		builder.WriteString("&")
	}

	hash := sha256.Sum256([]byte(builder.String()))
	return "async_samo:" + hex.EncodeToString(hash[:])
}

func (r *RedisCache) SetCachedStreamResult(
    ctx context.Context,
    key string,
    result *models.StreamCacheResult,
    ttl time.Duration,
) error {

    payload, err := json.Marshal(result)

    if err != nil {
        return err
    }

    return r.client.Set(
        ctx,
        key,
        payload,
        ttl,
    ).Err()
}


func BuildStreamCacheKey(
	params map[string]string,
) string {

	filtered := make(map[string]string)

	for k, v := range params {

		switch k {
		case "page",
			"page_size",
			"PRICEPAGE":
			continue
		}

		filtered[k] = v
	}

	b, _ := json.Marshal(filtered)

	hash := sha256.Sum256(b)

	return "stream:" +
		hex.EncodeToString(hash[:])
}