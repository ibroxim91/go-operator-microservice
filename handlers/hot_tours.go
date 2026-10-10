package handlers

import (
	"context"
	"net/http"
	"strings"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/scheduler"

	"github.com/labstack/echo/v4"
)

const hotToursFeaturedCount = 4
const hotToursPageSize = 100

func makeHotToursHandler(ctx context.Context, cacheClient *cache.RedisCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := strings.ToLower(strings.TrimSpace(c.QueryParam("scope")))
		if scope == "" {
			scope = "featured"
		}
		if scope != "featured" && scope != "rest" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "scope must be featured or rest"})
		}
		page := parsePageQuery(c.QueryParam("page"))

		cached, hit, err := cacheClient.LookupHomeOffersCache(ctx, cache.HotToursCacheKey)
		if err != nil {
			logger.Log.Warn().Err(err).Str("handler", "async-samo/hot-tours").Msg("failed to lookup hot tours cache")
		}
		if !hit || cached == nil {
			warmed, warmErr := scheduler.WarmHotToursCache(ctx, cacheClient)
			if warmErr != nil {
				logger.Log.Error().Err(warmErr).Str("handler", "async-samo/hot-tours").Msg("hot tours warmup failed")
				return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
			}
			cached = warmed
		}
		if cached == nil {
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
		}
		return c.JSON(http.StatusOK, sliceHotTours(cached, scope, page))
	}
}

func sliceHotTours(cached *models.AsyncSamoResult, scope string, page int) *models.AsyncSamoResult {
	tickets := cached.Data.Results.Tickets
	if scope == "featured" {
		if len(tickets) > hotToursFeaturedCount {
			tickets = tickets[:hotToursFeaturedCount]
		}
		page = 1
	} else if len(tickets) > hotToursFeaturedCount {
		tickets = tickets[hotToursFeaturedCount:]
	} else {
		tickets = []*models.Ticket{}
	}

	totalItems := len(tickets)
	start := (page - 1) * hotToursPageSize
	if start < 0 {
		start = 0
	}
	if start > totalItems {
		start = totalItems
	}
	end := start + hotToursPageSize
	if end > totalItems {
		end = totalItems
	}
	totalPages := 0
	if totalItems > 0 {
		totalPages = totalItems / hotToursPageSize
		if totalItems%hotToursPageSize != 0 {
			totalPages++
		}
	}

	pageTickets := tickets[start:end]
	for _, ticket := range pageTickets {
		if ticket != nil {
			ticket.FromCache = true
		}
	}

	out := *cached
	out.Data.TotalItems = totalItems
	out.Data.Total = totalItems
	out.Data.TotalPages = totalPages
	out.Data.PageSize = hotToursPageSize
	out.Data.CurrentPage = page
	out.Data.Results.Tickets = pageTickets
	out.Data.Results.RecommendedTickets = []*models.Ticket{}
	return &out
}
