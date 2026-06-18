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
	defaultPopularDestSchedulerInterval = 3 * time.Hour
	defaultPopularDestCollectWorkers    = 5
	popularDestSearchAdults             = "2"
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
		Str("cache_key", cache.PopularDestinationsCacheKey).
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
	perDestinationTickets := make([][]*models.Ticket, 0, len(destinations))
	var totalFound int
	var processedCount int
	var failedCount int

	for _, dest := range destinations {
		if ctx.Err() != nil {
			return
		}

		topTickets, foundCount, err := s.collectForDestination(ctx, dest, dateFrom, dateTo)
		if err != nil {
			failedCount++
			logger.Log.Warn().
				Err(err).
				Int("destination_id", dest.ID).
				Int("departure_region_id", dest.RegionID).
				Int("destination_region_id", dest.ToRegionID).
				Msg("failed to collect popular destination tickets")
			continue
		}

		if len(topTickets) > 0 {
			perDestinationTickets = append(perDestinationTickets, topTickets)
		}
		totalFound += foundCount
		processedCount++
	}

	if len(perDestinationTickets) == 0 {
		logger.Log.Warn().
			Int("destinations", len(destinations)).
			Int("failed", failedCount).
			Msg("popular destinations cache warmup produced no tickets")
		return
	}

	cacheResult := services.BuildPopularDestAsyncResult(perDestinationTickets, totalFound)
	if err := s.cacheClient.SetPopularDestCache(
		ctx,
		cache.PopularDestinationsCacheKey,
		cacheResult,
		cache.PopularDestCacheTTL,
	); err != nil {
		logger.Log.Error().
			Err(err).
			Str("key", cache.PopularDestinationsCacheKey).
			Msg("failed to save popular destinations cache")
		return
	}

	logger.Log.Info().
		Int("destinations", len(destinations)).
		Int("processed", processedCount).
		Int("failed", failedCount).
		Int("total_found", totalFound).
		Int("tickets", len(cacheResult.Data.Results.Tickets)).
		Str("key", cache.PopularDestinationsCacheKey).
		Str("date_from", dateFrom).
		Str("date_to", dateTo).
		Dur("duration", time.Since(startedAt)).
		Msg("popular destinations cache warmup completed")
}

func (s *PopularDestinationsScheduler) collectForDestination(
	ctx context.Context,
	dest repository.PopularDestination,
	dateFrom string,
	dateTo string,
) ([]*models.Ticket, int, error) {
	params := buildPopularDestSearchParams(dest, dateFrom, dateTo)

	jobs, err := s.samoService.MakeURLs(params)
	if err != nil {
		return nil, 0, err
	}
	for i := range jobs {
		jobs[i].FirstPageOnly = true
	}

	var tickets []*models.Ticket
	if len(jobs) > 0 {
		result := workers.CollectResults(ctx, jobs, s.workerCount, s.hotelService)
		tickets = result.Prices
	}

	topTickets := services.TakeCheapestTickets(tickets, 10)
	logger.Log.Info().
		Int("destination_id", dest.ID).
		Int("departure_region_id", dest.RegionID).
		Int("destination_region_id", dest.ToRegionID).
		Int("total_tickets", len(tickets)).
		Int("saved_tickets", len(topTickets)).
		Msg("popular destination tickets selected")

	return topTickets, len(tickets), nil
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
		"ADULT":           popularDestSearchAdults,
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
