package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-operator-service/logger"
	"go-operator-service/models"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

const popularDestCachePrefix = "popular_dest"

// PopularDestCacheTTL scheduler intervalidan uzunroq bo'lishi kerak.
const PopularDestCacheTTL = 25 * time.Minute

func BuildPopularDestCacheKey(departure, destination, adults string) string {
	if strings.TrimSpace(adults) == "" {
		adults = "1"
	}

	parts := []string{
		popularDestCachePrefix,
		strings.TrimSpace(departure),
		strings.TrimSpace(destination),
		strings.TrimSpace(adults),
	}
	return strings.Join(parts, ":")
}

func BuildPopularDestCacheKeyFromQuery(c echo.Context) string {
	adults := strings.TrimSpace(c.QueryParam("adults"))
	if adults == "" {
		adults = "1"
	}

	return BuildPopularDestCacheKey(
		c.QueryParam("departure"),
		c.QueryParam("destination"),
		adults,
	)
}

func IsPopularDestCacheEligible(c echo.Context) bool {
	filters := []string{
		"hotel_id",
		"hotel_rating",
		"rating",
		"meal",
		"meal_plan",
		"town",
		"min_price",
		"max_price",
		"operator",
		"duration",
		"duration_days",
	}
	for _, name := range filters {
		if isSet(c.QueryParam(name)) {
			return false
		}
	}

	children := strings.TrimSpace(c.QueryParam("children"))
	return children == "" || children == "0"
}

// ShouldUsePopularDestCache decides whether cached data can satisfy the request.
func ShouldUsePopularDestCache(userSpecifiedDate bool, dateFrom, dateTo string) bool {
	if !userSpecifiedDate {
		return true
	}
	return IsDateRangeInPopularDestWindow(dateFrom, dateTo)
}

func PopularDestCacheDateRange(now time.Time) (string, string) {
	tomorrow := truncateDay(now.AddDate(0, 0, 1))
	windowEnd := truncateDay(now.AddDate(0, 0, 31))
	return FormatPopularDestDate(tomorrow), FormatPopularDestDate(windowEnd)
}

func IsDateRangeInPopularDestWindow(dateFrom, dateTo string) bool {
	return isDateRangeInPopularDestWindow(dateFrom, dateTo)
}

func normalizePopularDestDate(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "-", "")
}

func isSet(value string) bool {
	return strings.TrimSpace(value) != ""
}

func isDateRangeInPopularDestWindow(dateFrom, dateTo string) bool {
	if dateFrom == "" || dateTo == "" {
		return false
	}

	from, err := parseYYYYMMDD(dateFrom)
	if err != nil {
		return false
	}
	to, err := parseYYYYMMDD(dateTo)
	if err != nil {
		return false
	}
	if from.After(to) {
		return false
	}

	now := time.Now()
	windowStart := truncateDay(now.AddDate(0, 0, 1))
	windowEnd := truncateDay(now.AddDate(0, 0, 31))

	return !from.Before(windowStart) && !to.After(windowEnd)
}

func parseYYYYMMDD(value string) (time.Time, error) {
	return time.ParseInLocation("20060102", normalizePopularDestDate(value), time.Local)
}

func truncateDay(value time.Time) time.Time {
	y, m, d := value.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, value.Location())
}

func FormatPopularDestDate(value time.Time) string {
	return value.Format("20060102")
}

func (r *RedisCache) GetPopularDestCache(
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
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *RedisCache) LookupPopularDestCache(
	ctx context.Context,
	key string,
) (*models.StreamCacheResult, bool, error) {
	cached, err := r.GetPopularDestCache(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if cached == nil {
		logger.Log.Info().Str("key", key).Msg("CACHE MISS")
		return nil, false, nil
	}

	logger.Log.Info().Str("key", key).Msg("CACHE HIT")
	return cached, true, nil
}

func (r *RedisCache) SetPopularDestCache(
	ctx context.Context,
	key string,
	result *models.StreamCacheResult,
	ttl time.Duration,
) error {
	if result == nil {
		return fmt.Errorf("popular destination cache result is nil")
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, payload, ttl).Err()
}
