package handlers

import (
	"context"
	"errors"

	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/workers"

	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Echo, ctx context.Context, hotelService *services.HotelService, samoService *services.SamoService, cacheClient *cache.RedisCache) {
	e.POST("/search-tours", makeSearchToursHandler(ctx, hotelService))
	e.GET("/async-samo/tickets", makeAsyncSamoTicketsHandler(ctx, hotelService, samoService, cacheClient))
	e.GET("/stream-samo/tickets", makeAsyncSamoTicketsStreamHandler(ctx, hotelService, samoService, cacheClient))
}

func makeSearchToursHandler(ctx context.Context, hotelService *services.HotelService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var jobs []models.Request
		if err := c.Bind(&jobs); err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("request bind failed")
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		results := workers.CollectResults(ctx, jobs, len(jobs), hotelService)
		return c.JSON(http.StatusOK, results)
	}
}

func makeAsyncSamoTicketsHandler(ctx context.Context, hotelService *services.HotelService, samoService *services.SamoService, cacheClient *cache.RedisCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		if token := strings.TrimSpace(c.QueryParam("shared_token")); token != "" {
			result, err := ResolveSharedTour(c.Request().Context(), cacheClient, hotelService, samoService, token)
			logger.Log.Println("Error ", err)
			if err != nil {
				switch {
				case errors.Is(err, ErrSharePayloadNotFound):
					return c.JSON(http.StatusNotFound, map[string]string{"error": "share link not found or expired"})
				case errors.Is(err, ErrShareTourNotFound):
					return c.JSON(http.StatusNotFound, map[string]string{"error": "tour not found for share link"})
				default:
					logger.Log.Error().
						Err(err).
						Str("handler", "async-samo/tickets").
						Str("shared_token", token).
						Msg("failed to resolve shared tour")
					return c.JSON(http.StatusBadGateway, map[string]string{"error": "failed to load shared tour"})
				}
			}
			return c.JSON(http.StatusOK, result)
		}

		samoParams, fromCache, userSpecifiedDate, err := samoService.GetSamoParams(c)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		if len(samoParams) == 0 {
			if fromCache {
				homeCache, err := cacheClient.GetHomeCache(ctx)
				if err != nil {
					logger.Log.Error().
						Err(err).
						Str("handler", "async-samo/tickets").
						Msg("failed to get home cache")
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get home cache"})
				}
				if homeCache != nil {
					return c.JSON(http.StatusOK, homeCache)
				}

				return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(1))
			}
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request parameters"})
		}
		log.Println("samoParams: ", samoParams)
		page := parseRequestedPage(samoParams)

		if cache.IsPopularDestCacheEligible(c) &&
			cache.ShouldUsePopularDestCache(userSpecifiedDate, samoParams["CHECKIN_BEG"], samoParams["CHECKIN_END"]) {
			if cachedResult, err := loadPopularDestAsyncResult(
				ctx,
				cacheClient,
				c,
				samoParams,
				userSpecifiedDate,
				page,
			); err != nil {
				logger.Log.Warn().
					Err(err).
					Str("handler", "async-samo/tickets").
					Msg("failed to load popular destination cache")
			} else if cachedResult != nil {
				return c.JSON(http.StatusOK, cachedResult)
			}
		}

		cacheKey := cache.GenerateCacheKey(samoParams)
		jobs, err := samoService.MakeURLs(samoParams)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "async-samo/tickets").
				Msg("failed to build request URLs")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to build request URLs"})
		}

		if len(jobs) == 0 {
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(1))
		}

		result, err := loadAsyncSamoTicketsResult(
			ctx,
			cacheClient,
			cacheKey,
			jobs,
			hotelService,
			samoParams,
			cache.IsPopularDestCacheEligible(c) &&
				cache.ShouldUsePopularDestCache(userSpecifiedDate, samoParams["CHECKIN_BEG"], samoParams["CHECKIN_END"]),
		)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "async-samo/tickets").
				Msg("failed to load async SAMO tickets result")
		}
		return c.JSON(http.StatusOK, result)
	}
}

func loadPopularDestAsyncResult(
	ctx context.Context,
	cacheClient *cache.RedisCache,
	c echo.Context,
	samoParams map[string]string,
	userSpecifiedDate bool,
	page int,
) (*models.AsyncSamoResult, error) {
	cached, hit, err := cacheClient.LookupPopularDestCache(ctx, cache.PopularDestinationsCacheKey)
	if err != nil {
		return nil, err
	}
	if !hit {
		return nil, nil
	}

	sliced := services.FilterPopularDestAsyncResult(
		cached,
		c.QueryParam("departure"),
		c.QueryParam("destination"),
		c.QueryParam("country_id"),
		userSpecifiedDate,
		samoParams["CHECKIN_BEG"],
		samoParams["CHECKIN_END"],
	)
	if sliced == nil || len(sliced.Data.Results.Tickets) == 0 {
		return nil, nil
	}

	return paginatePopularDestAsyncResult(ctx, cacheClient, sliced, page), nil
}

func paginatePopularDestAsyncResult(ctx context.Context, cacheClient *cache.RedisCache, response *models.AsyncSamoResult, page int) *models.AsyncSamoResult {
	totalFound := response.Data.TotalItems
	paginated := paginateAsyncSamoResult(ctx, cacheClient, response, page)
	paginated.Data.TotalItems = totalFound
	return paginated
}

func loadAsyncSamoTicketsResult(
	ctx context.Context,
	cacheClient *cache.RedisCache,
	cacheKey string,
	jobs []models.Request,
	hotelService *services.HotelService,
	samoParams map[string]string,
	skipLegacyCache bool,
) (*models.AsyncSamoResult, error) {
	page := parseRequestedPage(samoParams)

	if !skipLegacyCache {
		cachedResult, err := cacheClient.GetCachedAsyncResult(ctx, cacheKey)
		if err != nil {
			return nil, err
		}
		if cachedResult != nil {
			for _, ticket := range cachedResult.Data.Results.Tickets {
				ticket.FromCache = true
			}
			return paginateAsyncSamoResult(ctx, cacheClient, cachedResult, page), nil
		}
	}

	workerResult := workers.CollectResults(ctx, jobs, len(jobs), hotelService)
	response := services.BuildAsyncSamoResult(workerResult)

	if !skipLegacyCache && len(response.Data.Results.Tickets) > 0 {
		if err := cacheClient.SetCachedAsyncResult(ctx, cacheKey, response, 10*time.Minute); err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "async-samo/tickets").
				Msg("failed to cache async SAMO tickets result")
		}
	}

	return paginateAsyncSamoResult(ctx, cacheClient, response, page), nil
}

func parseRequestedPage(samoParams map[string]string) int {
	page := 1
	if samoParams == nil {
		return page
	}
	if ps, ok := samoParams["PRICEPAGE"]; ok && ps != "" {
		if p, err := strconv.Atoi(ps); err == nil && p > 0 {
			page = p
		}
	}
	return page
}



func paginateAsyncSamoResult(ctx context.Context, cacheClient *cache.RedisCache, response *models.AsyncSamoResult, page int) *models.AsyncSamoResult {
	const pageSize = 100

	fullTickets := response.Data.Results.Tickets
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

	response.Data.TotalItems = totalItems
	totalPages := 0
	if totalItems > 0 {
		totalPages = totalItems / pageSize
		if totalItems%pageSize != 0 {
			totalPages++
		}
	}

	response.Data.TotalPages = totalPages
	response.Data.PageSize = pageSize
	response.Data.CurrentPage = page
	response.Data.Results.Tickets = fullTickets[start:end]
	cache.ApplyShareTokensToTickets(ctx, cacheClient, response.Data.Results.Tickets)
	return response
}

func buildEmptyAsyncSamoResult(page int) *models.AsyncSamoResult {
	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Links:       models.Links{Previous: nil, Next: nil},
			TotalItems:  0,
			TotalPages:  0,
			PageSize:    100,
			Total:       0,
			CurrentPage: page,
			Results: models.AsyncSamoResultPayload{
				Tickets:             []*models.Ticket{},
				MinPrice:            0,
				MaxPrice:            0,
				Hotels:              []models.HotelSummary{},
				HotelAmenities:      []string{},
				HotelFeaturesByType: []string{},
				HotelTypes:          []string{},
				TopDestinations:     []string{},
				TopDuration:         []string{},
			},
		},
	}
}

func buildStreamPayloadFromAsyncCache(cached *models.AsyncSamoResult, page int) StreamPayload {
	if page <= 0 {
		page = 1
	}

	tickets := cached.Data.Results.Tickets
	start := (page - 1) * 100
	end := start + 100
	if start > len(tickets) {
		start = len(tickets)
	}
	if end > len(tickets) {
		end = len(tickets)
	}

	return StreamPayload{
		Prices:     tickets[start:end],
		Hotels:     cached.Data.Results.Hotels,
		End:        true,
		Total:      cached.Data.Total,
		TotalPages: (len(tickets) + 99) / 100,
		TotalItems: cached.Data.TotalItems,
		FromCache:  true,
	}
}

func buildStreamPayloadFromCache(cached *models.StreamCacheResult, page int) StreamPayload {
	if page <= 0 {
		page = 1
	}

	start := (page - 1) * 100
	end := start + 100
	if start > len(cached.Tickets) {
		start = len(cached.Tickets)
	}
	if end > len(cached.Tickets) {
		end = len(cached.Tickets)
	}

	return StreamPayload{
		Prices:     cached.Tickets[start:end],
		Hotels:     cached.Hotels,
		End:        true,
		Total:      cached.Total,
		TotalPages: (len(cached.Tickets) + 99) / 100,
		TotalItems: cached.Total,
		FromCache:  true,
	}
}

func writeStreamPayload(rw http.ResponseWriter, flusher http.Flusher, payload StreamPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	fmt.Fprintf(rw, "data: %s\n\n", jsonData)
	flusher.Flush()
	return nil
}
