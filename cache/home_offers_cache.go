package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go-operator-service/logger"
	"go-operator-service/models"

	"github.com/redis/go-redis/v9"
)

const HomeOffersCacheKey = "home_offers"

// HomeOffersCacheTTL matches the 3h scheduler interval with buffer.
const HomeOffersCacheTTL = 4 * time.Hour

func (r *RedisCache) GetHomeOffersCache(
	ctx context.Context,
	key string,
) (*models.AsyncSamoResult, error) {
	value, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var result models.AsyncSamoResult
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *RedisCache) LookupHomeOffersCache(
	ctx context.Context,
	key string,
) (*models.AsyncSamoResult, bool, error) {
	cached, err := r.GetHomeOffersCache(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if cached == nil {
		logger.Log.Info().Str("key", key).Msg("HOME OFFERS CACHE MISS")
		return nil, false, nil
	}
	logger.Log.Info().Str("key", key).Msg("HOME OFFERS CACHE HIT")
	return cached, true, nil
}

func (r *RedisCache) SetHomeOffersCache(
	ctx context.Context,
	key string,
	result *models.AsyncSamoResult,
	ttl time.Duration,
) error {
	if result == nil {
		return fmt.Errorf("home offers cache result is nil")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, payload, ttl).Err()
}
