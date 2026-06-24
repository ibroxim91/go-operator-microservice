package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/utils"
)

var (
	ErrSharePayloadNotFound = errors.New("share payload not found")
	ErrShareTourNotFound    = errors.New("tour not found for share token")
	ErrShareFetchFailed     = errors.New("failed to fetch tour from search url")
)

var shareHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	},
}

type shareSearchMeta struct {
	departure        string
	country          string
	imageURL         string
	operator         string
	currentUsdCourse float64
	destinationID    int
	departureID      int
	countryID        int
}

func ResolveSharedTour(
	ctx context.Context,
	cacheClient *cache.RedisCache,
	hotelService *services.HotelService,
	samoService *services.SamoService,
	token string,
) (*models.AsyncSamoResult, error) {
	payload, err := cache.GetSharePayload(ctx, cacheClient, token)
	if err != nil {
		return nil, err
	}
	if payload == nil || strings.TrimSpace(payload.SearchURL) == "" {
		return nil, ErrSharePayloadNotFound
	}

	meta := parseShareMetaFromURL(payload.SearchURL)
	if rate, err := samoService.GetCurrentUsdCourse(); err != nil {
		logger.Log.Warn().
			Err(err).
			Str("handler", "async-samo/tickets").
			Str("shared_token", token).
			Msg("failed to get usd rate for shared tour")
		meta.currentUsdCourse = 1.0
	} else {
		meta.currentUsdCourse = rate
	}

	operator := payload.Operator
	if operator == "" {
		operator = meta.operator
	}

	hotelService.BeginSearch()
	defer hotelService.EndSearch()

	parsed, err := fetchSearchTourPage(ctx, payload.SearchURL, 1)
	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "async-samo/tickets").
			Str("shared_token", token).
			Msg("failed to fetch share search page 1")
		return nil, fmt.Errorf("%w: %v", ErrShareFetchFailed, err)
	}

	if price := findPriceByTourID(parsed.SearchTour_PRICES.Prices, payload.TourID); price != nil {
		return buildSharedTicketResult(
			price, meta, operator, payload.SearchURL, token, hotelService,
			parsed.SearchTour_PRICES.Pager.Total,
		)
	}

	totalPages := parsed.SearchTour_PRICES.Pager.Total
	for page := 2; page <= totalPages; page++ {
		pageParsed, pageErr := fetchSearchTourPage(ctx, payload.SearchURL, page)
		if pageErr != nil {
			logger.Log.Warn().
				Err(pageErr).
				Int("page", page).
				Str("shared_token", token).
				Msg("failed to fetch share search page")
			continue
		}

		if price := findPriceByTourID(pageParsed.SearchTour_PRICES.Prices, payload.TourID); price != nil {
			return buildSharedTicketResult(
				price, meta, operator, payload.SearchURL, token, hotelService,
				totalPages,
			)
		}
	}

	return nil, ErrShareTourNotFound
}

func buildSharedTicketResult(
	price *models.Price,
	meta shareSearchMeta,
	operator string,
	searchURL string,
	token string,
	hotelService *services.HotelService,
	sourceTotalPages int,
) (*models.AsyncSamoResult, error) {
	ticket := utils.TransformSamoPriceToTicket(
		*price,
		meta.departure,
		operator,
		meta.country,
		meta.imageURL,
		meta.currentUsdCourse,
		meta.destinationID,
		meta.departureID,
		meta.countryID,
		hotelService,
		false,
		searchURL,
	)
	ticket.ShareToken = token
	return buildSingleTicketAsyncResult(ticket, sourceTotalPages), nil
}

func findPriceByTourID(prices []models.Price, tourID string) *models.Price {
	tourID = strings.TrimSpace(tourID)
	if tourID == "" {
		return nil
	}
	for i := range prices {
		if prices[i].FreightExternal == "Y" {
			continue
		}
		if prices[i].ID == tourID {
			return &prices[i]
		}
	}
	return nil
}

func fetchSearchTourPage(ctx context.Context, baseURL string, page int) (*models.SearchTourResponse, error) {
	pageURL, err := buildShareSearchPageURL(baseURL, page)
	if err != nil {
		return nil, err
	}

	testMode := os.Getenv("TEST") == "true"
	var req *http.Request

	if testMode {
		payload := map[string]string{"url": pageURL}
		bodyBytes, _ := json.Marshal(payload)
		testURL := os.Getenv("TEST_URL")
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, testURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, err
		}
	}

	resp, err := shareHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var parsed models.SearchTourResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func buildShareSearchPageURL(base string, page int) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("PRICEPAGE", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func parseShareMetaFromURL(searchURL string) shareSearchMeta {
	meta := shareSearchMeta{}
	parsed, err := url.Parse(searchURL)
	if err != nil {
		return meta
	}

	q := parsed.Query()
	meta.departure = firstNonEmpty(q.Get("departure_name"))
	meta.country = firstNonEmpty(q.Get("country_name"))
	meta.imageURL = firstNonEmpty(q.Get("destination_image_url"))
	meta.destinationID = parseShareInt(firstNonEmpty(q.Get("destination"), q.Get("TOWNS")))
	meta.departureID = parseShareInt(firstNonEmpty(q.Get("departure"), q.Get("TOWNFROMINC")))
	meta.countryID = parseShareInt(q.Get("STATEINC"))
	meta.operator = firstNonEmpty(q.Get("OPERATOR"))
	return meta
}

func buildSingleTicketAsyncResult(ticket *models.Ticket, sourceTotalPages int) *models.AsyncSamoResult {
	hotels := services.BuildHotelSummaries([]*models.Ticket{ticket})

	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Links:       models.Links{Previous: nil, Next: nil},
			TotalItems:  1,
			TotalPages:  1,
			PageSize:    1,
			Total:       sourceTotalPages,
			CurrentPage: 1,
			Results: models.AsyncSamoResultPayload{
				Tickets:             []*models.Ticket{ticket},
				MinPrice:            ticket.PriceFull,
				MaxPrice:            ticket.PriceFull,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseShareInt(value string) int {
	if value == "" {
		return 0
	}
	i, err := strconv.Atoi(value)
	if err != nil || i < 0 {
		return 0
	}
	return i
}
