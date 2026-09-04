package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"go-operator-service/cache"
	"go-operator-service/db"
	"go-operator-service/logger"
	"go-operator-service/scheduler"
	"go-operator-service/services"

	"github.com/labstack/echo/v4"
)

func makeHomeOffersHandler(
	ctx context.Context,
	hotelService *services.HotelService,
	samoService *services.SamoService,
	cacheClient *cache.RedisCache,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		hot := parseBoolQuery(c.QueryParam("hot"))
		visaRequiredParam := strings.TrimSpace(c.QueryParam("visa_required"))
		page := parsePageQuery(c.QueryParam("page"))

		var visaRequiredFilter *bool
		if visaRequiredParam != "" {
			v := parseBoolQuery(visaRequiredParam)
			visaRequiredFilter = &v
		}

		// Accept either hot=true or visa_required=false|true.
		if !hot && visaRequiredFilter == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "provide hot=true and/or visa_required=false|true",
			})
		}

		cached, hit, err := cacheClient.LookupHomeOffersCache(ctx, cache.HomeOffersCacheKey)
		if err != nil {
			logger.Log.Warn().
				Err(err).
				Str("handler", "async-samo/home-offers").
				Msg("failed to lookup home offers cache")
		}

		if !hit || cached == nil {
			if db.DB == nil {
				logger.Log.Warn().Msg("db handle unavailable for home offers warmup")
				return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
			}
			warmed, warmErr := scheduler.WarmHomeOffersCache(ctx, db.DB, samoService, cacheClient, hotelService)
			if warmErr != nil {
				logger.Log.Error().
					Err(warmErr).
					Str("handler", "async-samo/home-offers").
					Msg("home offers warmup failed")
				return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
			}
			cached = warmed
		}

		if cached == nil {
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
		}

		filtered := services.CloneHomeOffersResult(cached, visaRequiredFilter)
		if filtered == nil || len(filtered.Data.Results.Tickets) == 0 {
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
		}

		return c.JSON(http.StatusOK, paginateAsyncSamoResult(ctx, cacheClient, filtered, page))
	}
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func parsePageQuery(value string) int {
	page := 1
	if p, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && p > 0 {
		page = p
	}
	return page
}
