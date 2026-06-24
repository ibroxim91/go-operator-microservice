package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"crypto/sha1"
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

func BuildShareToken(payload models.SharePayload) string {
    source := payload.TourID 
    sum := sha1.Sum([]byte(source))
    return hex.EncodeToString(sum[:12])
}


func SaveSharePayload(
    ctx context.Context,
    cacheClient *RedisCache,
    payload models.SharePayload,
) (string, error) {

    token := BuildShareToken(payload)
    key := "share:" + token

    exists, err := cacheClient.client.Exists(ctx, key).Result()
    if err != nil {
        return "", err
    }

    if exists > 0 {
        return token, nil
    }

    data, err := json.Marshal(payload)
    if err != nil {
        return "", err
    }

    return token, cacheClient.client.Set(
        ctx,
        key,
        data,
        7*24*time.Hour,
    ).Err()
}

func ApplyShareTokensToTickets(
	ctx context.Context,
	cacheClient *RedisCache,
	tickets []*models.Ticket,
) {
	for _, ticket := range tickets {
		if ticket == nil || strings.TrimSpace(ticket.ShareToken) != "" {
			continue
		}

		searchURL := strings.TrimSpace(ticket.RequestUrl)
		if searchURL == "" {
			continue
		}

		payload := models.SharePayload{
			Operator:  ticket.Operator,
			TourID:    ticket.TourOperatorID,
			SearchURL: searchURL,
		}

		token, err := SaveSharePayload(ctx, cacheClient, payload)
		if err != nil || token == "" {
			continue
		}

		ticket.ShareToken = token
		ticket.Slug = ticket.Slug + "share" + token
		ticket.RequestUrl = ""
	}
}

func GetSharePayload(
    ctx context.Context,
    cacheClient *RedisCache,
    token string,
) (*models.SharePayload, error) {
    token = strings.TrimSpace(token)
    if token == "" {
        return nil, nil
    }

    key := "share:" + token
    value, err := cacheClient.client.Get(ctx, key).Result()
    if err != nil {
        if err == redis.Nil {
            return nil, nil
        }
        return nil, err
    }

    var payload models.SharePayload
    if err := json.Unmarshal([]byte(value), &payload); err != nil {
        return nil, err
    }
    return &payload, nil
}


func (r *RedisCache) GetHomeCache(ctx context.Context) (*models.AsyncSamoResult, error) {
	value, err := r.client.Get(ctx, "popular_destinations").Result()

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

const usdRateCacheKey = "usd_rate"

func (r *RedisCache) GetUsdRate(ctx context.Context) (float64, bool, error) {
	value, err := r.client.Get(ctx, usdRateCacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, false, nil
		}
		return 0, false, err
	}

	rate, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false, err
	}

	return rate, true, nil
}

func (r *RedisCache) SetUsdRate(ctx context.Context, rate float64, ttl time.Duration) error {
	return r.client.Set(
		ctx,
		usdRateCacheKey,
		strconv.FormatFloat(rate, 'f', -1, 64),
		ttl,
	).Err()
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
	log.Println("builder.String() ", builder.String())
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
