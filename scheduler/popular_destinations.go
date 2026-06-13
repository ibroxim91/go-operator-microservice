package scheduler

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/repository"
	"go-operator-service/services"
	"go-operator-service/workers"
)

const (
	defaultPopularDestSchedulerInterval = 20 * time.Minute
	defaultPopularDestCollectWorkers    = 5
)

type PopularDestinationsScheduler struct {
	db           *sql.DB
	samoService  *services.SamoService
	cacheClient  *cache.RedisCache
	hotelService *services.HotelService
	workerCount  int
	interval     time.Duration
}

func StartPopularDestinationsScheduler(
	ctx context.Context,
	db *sql.DB,
	samoService *services.SamoService,
	cacheClient *cache.RedisCache,
	hotelService *services.HotelService,
) {
	scheduler := &PopularDestinationsScheduler{
		db:           db,
		samoService:  samoService,
		cacheClient:  cacheClient,
		hotelService: hotelService,
		workerCount:  popularDestCollectWorkerCount(),
		interval:     popularDestSchedulerInterval(),
	}

	logger.Log.Info().
		Dur("interval", scheduler.interval).
		Int("workers", scheduler.workerCount).
		Msg("popular destinations cache scheduler started")

	scheduler.runOnce(ctx)

	ticker := time.NewTicker(scheduler.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info().Msg("popular destinations cache scheduler stopped")
			return
		case <-ticker.C:
			scheduler.runOnce(ctx)
		}
	}
}

func (s *PopularDestinationsScheduler) runOnce(ctx context.Context) {
	startedAt := time.Now()

	destinations, err := repository.GetPopularDestinations(s.db)
	if err != nil {
		logger.Log.Error().
			Err(err).
			Msg("failed to load popular destinations for cache warmup")
		return
	}
	if len(destinations) == 0 {
		logger.Log.Warn().Msg("no popular destinations found for cache warmup")
		return
	}

	dateFrom, dateTo := cache.PopularDestCacheDateRange(time.Now())
	var savedCount int
	var failedCount int

	for _, dest := range destinations {
		if ctx.Err() != nil {
			return
		}

		if err := s.warmDestination(ctx, dest, dateFrom, dateTo); err != nil {
			failedCount++
			logger.Log.Warn().
				Err(err).
				Int("destination_id", dest.ID).
				Int("departure_region_id", dest.RegionID).
				Int("destination_region_id", dest.ToRegionID).
				Msg("failed to warm popular destination cache")
			continue
		}
		savedCount++
	}

	logger.Log.Info().
		Int("destinations", len(destinations)).
		Int("saved", savedCount).
		Int("failed", failedCount).
		Str("date_from", dateFrom).
		Str("date_to", dateTo).
		Dur("duration", time.Since(startedAt)).
		Msg("popular destinations cache warmup completed")
}

func (s *PopularDestinationsScheduler) warmDestination(
	ctx context.Context,
	dest repository.PopularDestination,
	dateFrom string,
	dateTo string,
) error {
	params := buildPopularDestSearchParams(dest, dateFrom, dateTo)

	jobs, err := s.samoService.MakeURLs(params)
	if err != nil {
		return err
	}

	var tickets []*models.Ticket
	if len(jobs) > 0 {
		result := workers.CollectResults(ctx, jobs, s.workerCount, s.hotelService)
		tickets = result.Prices
	}

	cacheResult := services.BuildStreamCacheResult(tickets)
	key := cache.BuildPopularDestCacheKey(
		strconv.Itoa(dest.RegionID),
		strconv.Itoa(dest.ToRegionID),
		"1",
	)

	if err := s.cacheClient.SetPopularDestCache(ctx, key, cacheResult, cache.PopularDestCacheTTL); err != nil {
		return err
	}

	logger.Log.Info().
		Str("key", key).
		Int("tickets", len(tickets)).
		Msg("popular destination cache saved")

	return nil
}

func buildPopularDestSearchParams(
	dest repository.PopularDestination,
	dateFrom string,
	dateTo string,
) map[string]string {
	return map[string]string{
		"samo_action":     "api",
		"version":         "1.0",
		"type":            "json",
		"action":          "SearchTour_PRICES",
		"OPERATOR":        "",
		"PRICEPAGE":       "1",
		"ADULT":           "1",
		"CHILD":           "0",
		"CURRENCY":        "2",
		"CHECKIN_BEG":     dateFrom,
		"CHECKIN_END":     dateTo,
		"NIGHTS_FROM":     "",
		"NIGHTS_LIST":     "2,3,4,5,6,7",
		"NIGHTS_TILL":     "",
		"SORT":            "ASC",
		"TOWNFROMINC":     strconv.Itoa(dest.RegionID),
		"STATEINC":        strconv.Itoa(dest.CountryID),
		"destination":     strconv.Itoa(dest.ToRegionID),
		"departure_name":  dest.DepartureName,
		"country_name":    dest.CountryName,
		"region__name":    dest.DestinationName,
		"region__name_uz": "",
		"region_id":       strconv.Itoa(dest.ToRegionID),
		"test":            os.Getenv("TEST"),
	}
}

func popularDestSchedulerInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("POPULAR_DEST_CACHE_INTERVAL_MINUTES"))
	if raw == "" {
		return defaultPopularDestSchedulerInterval
	}

	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes < 1 {
		return defaultPopularDestSchedulerInterval
	}

	return time.Duration(minutes) * time.Minute
}

func popularDestCollectWorkerCount() int {
	raw := strings.TrimSpace(os.Getenv("POPULAR_DEST_COLLECT_WORKERS"))
	if raw == "" {
		return defaultPopularDestCollectWorkers
	}

	count, err := strconv.Atoi(raw)
	if err != nil || count < 1 {
		return defaultPopularDestCollectWorkers
	}

	return count
}
