package scheduler

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/repository"
	"go-operator-service/services"
	"go-operator-service/workers"
)

const (
	defaultHomeOffersSchedulerInterval = 3 * time.Hour
	defaultHomeOffersCollectWorkers    = 5
	homeOffersSearchAdults             = "1"
)

var homeOffersWarmupMu sync.Mutex

type HomeOffersScheduler struct {
	db           *sql.DB
	samoService  *services.SamoService
	cacheClient  *cache.RedisCache
	hotelService *services.HotelService
	workerCount  int
	interval     time.Duration
}

func StartHomeOffersScheduler(
	ctx context.Context,
	db *sql.DB,
	samoService *services.SamoService,
	cacheClient *cache.RedisCache,
	hotelService *services.HotelService,
) {
	scheduler := &HomeOffersScheduler{
		db:           db,
		samoService:  samoService,
		cacheClient:  cacheClient,
		hotelService: hotelService,
		workerCount:  homeOffersCollectWorkerCount(),
		interval:     homeOffersSchedulerInterval(),
	}

	logger.Log.Info().
		Dur("interval", scheduler.interval).
		Int("workers", scheduler.workerCount).
		Str("cache_key", cache.HomeOffersCacheKey).
		Msg("home offers cache scheduler started")

	scheduler.runOnce(ctx)

	ticker := time.NewTicker(scheduler.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info().Msg("home offers cache scheduler stopped")
			return
		case <-ticker.C:
			scheduler.runOnce(ctx)
		}
	}
}

// WarmHomeOffersCache rebuilds cache if missing (request-time fallback).
func WarmHomeOffersCache(
	ctx context.Context,
	db *sql.DB,
	samoService *services.SamoService,
	cacheClient *cache.RedisCache,
	hotelService *services.HotelService,
) (*models.AsyncSamoResult, error) {
	homeOffersWarmupMu.Lock()
	defer homeOffersWarmupMu.Unlock()

	if cached, hit, err := cacheClient.LookupHomeOffersCache(ctx, cache.HomeOffersCacheKey); err == nil && hit && cached != nil {
		return cached, nil
	}

	scheduler := &HomeOffersScheduler{
		db:           db,
		samoService:  samoService,
		cacheClient:  cacheClient,
		hotelService: hotelService,
		workerCount:  homeOffersCollectWorkerCount(),
		interval:     homeOffersSchedulerInterval(),
	}
	return scheduler.buildAndStore(ctx)
}

func (s *HomeOffersScheduler) runOnce(ctx context.Context) {
	homeOffersWarmupMu.Lock()
	defer homeOffersWarmupMu.Unlock()
	if _, err := s.buildAndStore(ctx); err != nil {
		logger.Log.Error().Err(err).Msg("home offers cache warmup failed")
	}
}

func (s *HomeOffersScheduler) buildAndStore(ctx context.Context) (*models.AsyncSamoResult, error) {
	startedAt := time.Now()
	services.RefreshCountryVisaMap(s.db)

	destinations, err := repository.GetPopularDestinations(s.db)
	if err != nil {
		return nil, err
	}
	if len(destinations) == 0 {
		logger.Log.Warn().Msg("no popular destinations found for home offers warmup")
		return nil, nil
	}

	dateFrom, dateTo := cache.PopularDestCacheDateRange(time.Now())
	allTickets := make([]*models.Ticket, 0)
	var totalFound int
	var processedCount int
	var failedCount int

	for _, dest := range destinations {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		tickets, foundCount, err := s.collectForDestination(ctx, dest, dateFrom, dateTo)
		if err != nil {
			failedCount++
			logger.Log.Warn().
				Err(err).
				Int("destination_id", dest.ID).
				Msg("failed to collect home offers destination tickets")
			continue
		}
		allTickets = append(allTickets, tickets...)
		totalFound += foundCount
		processedCount++
	}

	if len(allTickets) == 0 {
		logger.Log.Warn().
			Int("destinations", len(destinations)).
			Int("failed", failedCount).
			Msg("home offers cache warmup produced no tickets")
		return nil, nil
	}

	cacheResult := services.BuildHomeOffersAsyncResult(allTickets, totalFound)
	cache.ApplyShareTokensToTickets(ctx, s.cacheClient, cacheResult.Data.Results.Tickets)
	if err := s.cacheClient.SetHomeOffersCache(
		ctx,
		cache.HomeOffersCacheKey,
		cacheResult,
		cache.HomeOffersCacheTTL,
	); err != nil {
		return nil, err
	}

	logger.Log.Info().
		Int("destinations", len(destinations)).
		Int("processed", processedCount).
		Int("failed", failedCount).
		Int("total_found", totalFound).
		Int("tickets", len(cacheResult.Data.Results.Tickets)).
		Str("key", cache.HomeOffersCacheKey).
		Str("date_from", dateFrom).
		Str("date_to", dateTo).
		Dur("duration", time.Since(startedAt)).
		Msg("home offers cache warmup completed")

	return cacheResult, nil
}

func (s *HomeOffersScheduler) collectForDestination(
	ctx context.Context,
	dest repository.PopularDestination,
	dateFrom string,
	dateTo string,
) ([]*models.Ticket, int, error) {
	params := buildHomeOffersSearchParams(dest, dateFrom, dateTo)

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

	logger.Log.Info().
		Int("destination_id", dest.ID).
		Int("departure_region_id", dest.RegionID).
		Int("destination_region_id", dest.ToRegionID).
		Int("total_tickets", len(tickets)).
		Msg("home offers destination tickets collected")

	return tickets, len(tickets), nil
}

func buildHomeOffersSearchParams(
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
		"ADULT":           homeOffersSearchAdults,
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

func homeOffersSchedulerInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("HOME_OFFERS_CACHE_INTERVAL_MINUTES"))
	if raw == "" {
		return defaultHomeOffersSchedulerInterval
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes < 1 {
		return defaultHomeOffersSchedulerInterval
	}
	return time.Duration(minutes) * time.Minute
}

func homeOffersCollectWorkerCount() int {
	raw := strings.TrimSpace(os.Getenv("HOME_OFFERS_COLLECT_WORKERS"))
	if raw == "" {
		return defaultHomeOffersCollectWorkers
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 1 {
		return defaultHomeOffersCollectWorkers
	}
	return count
}
