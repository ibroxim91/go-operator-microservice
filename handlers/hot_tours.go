package handlers

import (
	"context"
	"net/http"
	"strings"

	"go-operator-service/cache"
	"go-operator-service/db"
	"go-operator-service/hottours"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/scheduler"
	"go-operator-service/services"

	"github.com/labstack/echo/v4"
)

const hotToursFeaturedCount = 4
const hotToursPageSize = 100

func makeHotToursHandler(
	ctx context.Context,
	hotelService *services.HotelService,
	samoService *services.SamoService,
	cacheClient *cache.RedisCache,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := strings.ToLower(strings.TrimSpace(c.QueryParam("scope")))
		if scope == "" {
			scope = "featured"
		}
		if scope != "featured" && scope != "rest" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "scope must be featured or rest"})
		}
		page := parsePageQuery(c.QueryParam("page"))
		source := hottours.Source()

		cached, err := loadHotToursCache(ctx, hotelService, samoService, cacheClient, source)
		if err != nil {
			logger.Log.Error().Err(err).Str("source", source).Str("handler", "async-samo/hot-tours").Msg("hot tours load failed")
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
		}
		if cached == nil {
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(page))
		}
		// Tourvisor homepage cards are excluded from the list; SAMO rest is the full cheapest list.
		skipFeaturedOnRest := source == hottours.SourceTourvisor
		return c.JSON(http.StatusOK, sliceHotTours(cached, scope, page, skipFeaturedOnRest))
	}
}

func loadHotToursCache(
	ctx context.Context,
	hotelService *services.HotelService,
	samoService *services.SamoService,
	cacheClient *cache.RedisCache,
	source string,
) (*models.AsyncSamoResult, error) {
	if source == hottours.SourceTourvisor {
		cached, hit, err := cacheClient.LookupHomeOffersCache(ctx, cache.HotToursCacheKey)
		if err != nil {
			logger.Log.Warn().Err(err).Str("handler", "async-samo/hot-tours").Msg("failed to lookup tourvisor hot tours cache")
		}
		if hit && cached != nil {
			return cached, nil
		}
		return scheduler.WarmHotToursCache(ctx, cacheClient)
	}

	cached, hit, err := cacheClient.LookupHomeOffersCache(ctx, cache.HomeOffersCacheKey)
	if err != nil {
		logger.Log.Warn().Err(err).Str("handler", "async-samo/hot-tours").Msg("failed to lookup samo home-offers cache")
	}
	if hit && cached != nil {
		return services.CloneHomeOffersResult(cached, nil), nil
	}
	if db.DB == nil {
		return nil, nil
	}
	warmed, warmErr := scheduler.WarmHomeOffersCache(ctx, db.DB, samoService, cacheClient, hotelService)
	if warmErr != nil {
		return nil, warmErr
	}
	return services.CloneHomeOffersResult(warmed, nil), nil
}

func sliceHotTours(cached *models.AsyncSamoResult, scope string, page int, skipFeaturedOnRest bool) *models.AsyncSamoResult {
	tickets := cached.Data.Results.Tickets
	if scope == "featured" {
		if len(tickets) > hotToursFeaturedCount {
			tickets = tickets[:hotToursFeaturedCount]
		}
		page = 1
	} else if skipFeaturedOnRest {
		if len(tickets) > hotToursFeaturedCount {
			tickets = tickets[hotToursFeaturedCount:]
		} else {
			tickets = []*models.Ticket{}
		}
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
