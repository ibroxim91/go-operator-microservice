package scheduler

import (
	"context"
	"os"
	"strconv"
	"time"

	"sync"

	"go-operator-service/cache"
	"go-operator-service/hottours"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
)

var hotToursWarmupMu sync.Mutex

const hotToursCacheInterval = 3 * time.Hour

func StartHotToursScheduler(ctx context.Context, cacheClient *cache.RedisCache) {
	interval := hotToursInterval()
	logger.Log.Info().Dur("interval", interval).Msg("hot tours scheduler started")
	runHotToursRefresh(ctx, cacheClient)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Log.Info().Msg("hot tours scheduler stopped")
			return
		case <-ticker.C:
			runHotToursRefresh(ctx, cacheClient)
		}
	}
}

func hotToursInterval() time.Duration {
	raw := os.Getenv("HOT_TOURS_CACHE_INTERVAL_MINUTES")
	if raw == "" {
		return hotToursCacheInterval
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes <= 0 {
		return hotToursCacheInterval
	}
	return time.Duration(minutes) * time.Minute
}

// WarmHotToursCache rebuilds the hot tours cache when a request finds it empty.
func WarmHotToursCache(ctx context.Context, cacheClient *cache.RedisCache) (*models.AsyncSamoResult, error) {
	hotToursWarmupMu.Lock()
	defer hotToursWarmupMu.Unlock()

	if cached, hit, err := cacheClient.LookupHomeOffersCache(ctx, cache.HotToursCacheKey); err == nil && hit && cached != nil {
		return cached, nil
	}
	return refreshHotTours(ctx, cacheClient)
}

func runHotToursRefresh(ctx context.Context, cacheClient *cache.RedisCache) {
	if _, err := refreshHotTours(ctx, cacheClient); err != nil {
		logger.Log.Error().Err(err).Msg("hot tours refresh failed")
	}
}

func refreshHotTours(ctx context.Context, cacheClient *cache.RedisCache) (*models.AsyncSamoResult, error) {
	currency := services.NewCurrencyService(cacheClient)
	course, err := currency.GetUsdRate(ctx)
	if err != nil {
		return nil, err
	}
	result, err := hottours.FetchHotTours(ctx, course)
	if err != nil {
		return nil, err
	}
	if err := cacheClient.SetHomeOffersCache(ctx, cache.HotToursCacheKey, result, cache.HomeOffersCacheTTL); err != nil {
		return nil, err
	}
	logger.Log.Info().Int("tickets", result.Data.TotalItems).Msg("hot tours cache refreshed")
	return result, nil
}
