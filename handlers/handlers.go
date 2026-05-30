package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
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
		samoParams, fromCache, err := samoService.GetSamoParams(c)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if len(samoParams) == 0 {
			if fromCache {
				return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(1))
			}
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request parameters"})
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

		result, err := loadAsyncSamoTicketsResult(ctx, cacheClient, cacheKey, jobs, hotelService, samoParams)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "async-samo/tickets").
				Msg("failed to load async SAMO tickets result")
		}
		return c.JSON(http.StatusOK, result)
	}
}

func loadAsyncSamoTicketsResult(ctx context.Context, cacheClient *cache.RedisCache, cacheKey string, jobs []models.Request, hotelService *services.HotelService, samoParams map[string]string) (*models.AsyncSamoResult, error) {
	// determine requested page (default 1)
	page := 1
	if samoParams != nil {
		if ps, ok := samoParams["PRICEPAGE"]; ok && ps != "" {
			if p, err := strconv.Atoi(ps); err == nil && p > 0 {
				page = p
			}
		}
	}
	const pageSize = 100

	cachedResult, err := cacheClient.GetCachedAsyncResult(ctx, cacheKey)
	if err != nil {
		return nil, err
	}
	log.Println("samoParams ", samoParams)
	if cachedResult != nil {
		// mark all returned tickets as from cache
		fullTickets := cachedResult.Data.Results.Tickets
		for _, ticket := range fullTickets {
			ticket.FromCache = true
		}

		// paginate
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
		paged := fullTickets[start:end]

		cachedResult.Data.TotalItems = totalItems
		// compute pages
		totalPages := 0
		if totalItems > 0 {
			totalPages = totalItems / pageSize
			if totalItems%pageSize != 0 {
				totalPages++
			}
		}
		cachedResult.Data.TotalPages = totalPages
		cachedResult.Data.PageSize = pageSize
		cachedResult.Data.CurrentPage = page
		cachedResult.Data.Results.Tickets = paged

		return cachedResult, nil
	}

	workerResult := workers.CollectResults(ctx, jobs, len(jobs), hotelService)
	response := buildAsyncSamoResult(workerResult)

	// cache full response if any tickets
	if len(response.Data.Results.Tickets) > 0 {
		if err := cacheClient.SetCachedAsyncResult(ctx, cacheKey, response, 10*time.Minute); err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "async-samo/tickets").
				Msg("failed to cache async SAMO tickets result")
			// log.Printf("Failed to cache async SAMO tickets result: %v", err) --- IGNORE ---
		}
	}

	// apply pagination to response regardless of caching
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
	paged := fullTickets[start:end]

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
	response.Data.Results.Tickets = paged

	return response, nil
}

func buildAsyncSamoResult(results models.ResultResponse) *models.AsyncSamoResult {
	tickets := make([]*models.Ticket, len(results.Prices))
	copy(tickets, results.Prices)
	sort.Slice(tickets, func(i, j int) bool {
		return tickets[i].PriceFull < tickets[j].PriceFull
	})

	minPrice := 0
	maxPrice := 0
	if len(tickets) > 0 {
		minPrice = tickets[0].PriceFull
		maxPrice = tickets[0].PriceFull
		for _, ticket := range tickets {
			if ticket.PriceFull < minPrice {
				minPrice = ticket.PriceFull
			}
			if ticket.PriceFull > maxPrice {
				maxPrice = ticket.PriceFull
			}
		}
	}

	hotelMap := map[string]models.HotelSummary{}
	hotels := make([]models.HotelSummary, 0)
	for _, ticket := range tickets {
		for _, hotel := range ticket.TicketHotel {
			key := fmt.Sprintf("%d|%s", hotel.ID, hotel.Name)
			if _, ok := hotelMap[key]; ok {
				continue
			}
			hotelMap[key] = models.HotelSummary{
				ID:          hotel.ID,
				Name:        hotel.Name,
				MealPlan:    hotel.MealPlan,
				Rating:      hotel.Rating,
				Operator:    ticket.Operator,
				Destination: ticket.Destination.Name,
			}
			hotels = append(hotels, hotelMap[key])
		}
	}

	pageSize := 100
	totalItems := len(tickets)
	if totalItems == 0 {
		totalItems = 0
	}
	page := results.Page
	if page == 0 {
		page = 1
	}

	pages := 0
	if totalItems > 0 {
		pages = totalItems / pageSize
		if totalItems%pageSize != 0 {
			pages++
		}
	}

	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Links:       models.Links{Previous: nil, Next: nil},
			TotalItems:  totalItems,
			TotalPages:  pages,
			PageSize:    pageSize,
			Total:       results.Total,
			CurrentPage: page,
			Results: models.AsyncSamoResultPayload{
				Tickets:             tickets,
				MinPrice:            minPrice,
				MaxPrice:            maxPrice,
				Hotels:              hotels,
				HotelAmenities:      []string{},
				HotelFeaturesByType: []string{},
				HotelTypes:          []string{},
				TopDestinations:     []string{},
				TopDuration:         []string{},
			},
		},
	}
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
