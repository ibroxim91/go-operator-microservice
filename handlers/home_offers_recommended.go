package handlers

import (
	"context"
	"net/http"

	"go-operator-service/cache"
	"go-operator-service/db"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/scheduler"
	"go-operator-service/services"

	"github.com/labstack/echo/v4"
)

func makeHomeOffersRecommendedHandler(
	ctx context.Context,
	hotelService *services.HotelService,
	samoService *services.SamoService,
	cacheClient *cache.RedisCache,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		page := parsePageQuery(c.QueryParam("page"))

		cached, hit, err := cacheClient.LookupHomeOffersCache(ctx, cache.HomeOffersRecommendedCacheKey)
		if err != nil {
			logger.Log.Warn().
				Err(err).
				Str("handler", "async-samo/home-offers/recommended").
				Msg("failed to lookup recommended home offers cache")
		}

		if !hit || cached == nil {
			if db.DB == nil {
				logger.Log.Warn().Msg("db handle unavailable for recommended home offers warmup")
				return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
			}
			warmed, warmErr := scheduler.WarmRecommendedHomeOffersCache(
				ctx, db.DB, samoService, cacheClient, hotelService,
			)
			if warmErr != nil {
				logger.Log.Error().
					Err(warmErr).
					Str("handler", "async-samo/home-offers/recommended").
					Msg("recommended home offers warmup failed")
				return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
			}
			cached = warmed
		}

		if cached == nil || len(cached.Data.Results.Tickets) == 0 {
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
		}

		// Paginate without re-sorting / clearing is_recommended (already curated in cache).
		return c.JSON(http.StatusOK, paginateRecommendedHomeOffers(ctx, cacheClient, cached, page))
	}
}

func cloneAsyncSamoResult(cached *models.AsyncSamoResult) *models.AsyncSamoResult {
	if cached == nil {
		return nil
	}
	tickets := make([]*models.Ticket, len(cached.Data.Results.Tickets))
	copy(tickets, cached.Data.Results.Tickets)
	out := *cached
	out.Data.Results.Tickets = tickets
	return &out
}

func paginateRecommendedHomeOffers(
	ctx context.Context,
	cacheClient *cache.RedisCache,
	cached *models.AsyncSamoResult,
	page int,
) *models.AsyncSamoResult {
	const pageSize = 100
	response := cloneAsyncSamoResult(cached)
	if response == nil {
		return buildEmptyAsyncSamoResult(page)
	}

	fullTickets := response.Data.Results.Tickets
	for _, ticket := range fullTickets {
		if ticket != nil {
			ticket.FromCache = true
			ticket.IsRecommended = true
		}
	}

	totalItems := len(fullTickets)
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if start > totalItems {
		start = totalItems
	}
	if end > totalItems {
		end = totalItems
	}

	totalPages := 0
	if totalItems > 0 {
		totalPages = totalItems / pageSize
		if totalItems%pageSize != 0 {
			totalPages++
		}
	}

	response.Data.TotalItems = totalItems
	response.Data.TotalPages = totalPages
	response.Data.PageSize = pageSize
	response.Data.CurrentPage = page
	response.Data.Total = totalItems
	response.Data.Results.Tickets = fullTickets[start:end]
	response.Data.Results.RecommendedTickets = fullTickets[start:end]
	response.Data.Results.Hotels = services.BuildHotelSummaries(response.Data.Results.Tickets)
	cache.ApplyShareTokensToTickets(ctx, cacheClient, response.Data.Results.Tickets)
	return response
}
