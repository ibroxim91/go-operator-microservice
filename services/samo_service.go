package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/repository"

	"github.com/labstack/echo/v4"
)

type SamoService struct {
	DB              *sql.DB
	currencyService *CurrencyService
}

type SamoServiceConfig struct {
	Name       string
	BaseURL    string
	OAuthToken string
}

func NewSamoService(db *sql.DB, cacheClient *cache.RedisCache) *SamoService {
	return &SamoService{
		DB:              db,
		currencyService: NewCurrencyService(cacheClient),
	}
}

func (s *SamoService) getServiceConfigs() []SamoServiceConfig {
	services := []SamoServiceConfig{
		{
			Name:       "easy_booking",
			BaseURL:    os.Getenv("EASY_BOOKING_BASE_URL"),
			OAuthToken: os.Getenv("EASY_BOOKING_OAUTH_TOKEN"),
		},
		{
			Name:       "samo_tour",
			BaseURL:    os.Getenv("SAMO_TOUR_BASE_URL"),
			OAuthToken: os.Getenv("SAMO_TOUR_OAUTH_TOKEN"),
		},
		{
			Name:       "flykhiva",
			BaseURL:    os.Getenv("FLYKHIVA_BASE_URL"),
			OAuthToken: os.Getenv("FLYKHIVA_OAUTH_TOKEN"),
		},
		{
			Name:       "malva_tour",
			BaseURL:    os.Getenv("MALVA_TOUR_BASE_URL"),
			OAuthToken: os.Getenv("MALVA_TOUR_OAUTH_TOKEN"),
		},
		{
			Name:       "aqua_travelplus",
			BaseURL:    os.Getenv("AQUA_TRAVELPLUS_BASE_URL"),
			OAuthToken: os.Getenv("AQUA_TRAVELPLUS_OAUTH_TOKEN"),
		},
	}
	return services

}

func (s *SamoService) GetSamoParams(c echo.Context) (map[string]string, bool, bool, error) {
	getTrimmed := func(name, def string) string {
		value := strings.TrimSpace(c.QueryParam(name))
		if value == "" {
			return def
		}
		return value
	}

	page := getTrimmed("page", "1")
	countryID := getTrimmed("country_id", "")
	fromCache := strings.EqualFold(getTrimmed("from_cache", "false"), "true")
	town := getTrimmed("town", "")
	rawDateFrom := strings.TrimSpace(c.QueryParam("dateFrom"))
	rawDateTo := strings.TrimSpace(c.QueryParam("dateTo"))
	userSpecifiedDate := rawDateFrom != "" || rawDateTo != ""
	dateFrom := formatDate(rawDateFrom)
	dateTo := formatDate(rawDateTo)
	adults := getTrimmed("adults", "1")
	children := getTrimmed("children", "0")
	operator := getTrimmed("operator", "")
	departure := getTrimmed("departure", "")
	destination := getTrimmed("destination", "")
	countryName := ""
	regionName := ""
	regionNameUz := ""
	regionID := ""
	// minDepartureDate := formatDate(getTrimmed("min_departure_date", ""))
	// maxDepartureDate := formatDate(getTrimmed("max_departure_date", ""))
	minPrice := getTrimmed("min_price", "")
	maxPrice := getTrimmed("max_price", "")
	hotelRating := getTrimmed("hotel_rating", "")
	rating := getTrimmed("rating", "")
	durationDays := getTrimmed("duration_days", "")
	duration := getTrimmed("duration", "")
	mealPlan := getTrimmed("meal_plan", "")
	meal := getTrimmed("meal", "")
	hotelID := getTrimmed("hotel_id", "")
	currentUsdCourse := getTrimmed("current_usd_course", "")
	cheapest := strings.EqualFold(getTrimmed("cheapest", "false"), "true")
	mostExpensive := strings.EqualFold(getTrimmed("most_expensive", "false"), "true")
	recommendedOnly := strings.EqualFold(getTrimmed("recommended", "false"), "true")
	if recommendedOnly {
		cheapest = false
		mostExpensive = false
	}

	if adults == "" || parsePositiveInt(adults) <= 0 {
		adults = "1"
	}
	if children == "" || parsePositiveInt(children) < 0 {
		children = "0"
	}
	if countryID == "" && destination != "" {
		destinationID := parsePositiveInt(destination)
		if destinationID > 0 {
			regionInfo, err := repository.GetRegionByID(s.DB, destinationID)
			if err != nil && err != sql.ErrNoRows {
				return nil, false, false, err
			}
			if regionInfo != nil {
				countryID = strconv.Itoa(regionInfo.CountryID)
				countryName = regionInfo.CountryNameRu
				regionName = regionInfo.NameRu
				regionNameUz = regionInfo.NameUz
				regionID = strconv.Itoa(regionInfo.ID)
			}
		}
	}
	departure_name := ""
	if departure != "" {
		departureID := parsePositiveInt(departure)
		if departureID > 0 {
			regionInfo, err := repository.GetRegionByID(s.DB, departureID)
			if err != nil && err != sql.ErrNoRows {
				if regionInfo != nil {
					departure_name = regionInfo.NameRu
				}
			}

		}
	}

	log.Println("CountryId ", countryID, " | Destination ", destination, " | Town ", town, " | Departure ", departure, " | From cache ", fromCache)

	if departure == "" || countryID == "" {
		log.Println("Missing required departure or country_id parameters - departure: ", departure, " | country_id: ", countryID)
		return map[string]string{}, true, false, nil

		// return nil, false, errors.New("missing required departure or country_id")
	}

	today := time.Now()
	checkinBeg := today.Add(3 * 24 * time.Hour).Format("20060102")
	checkinEnd := today.Add(10 * 24 * time.Hour).Format("20060102")

	params := map[string]string{
		"samo_action":     "api",
		"version":         "1.0",
		"type":            "json",
		"action":          "SearchTour_PRICES",
		"OPERATOR":        operator,
		"PRICEPAGE":       page,
		"ADULT":           adults,
		"CHILD":           children,
		"CURRENCY":        "2",
		"CHECKIN_BEG":     checkinBeg,
		"CHECKIN_END":     checkinEnd,
		"NIGHTS_FROM":     "",
		"NIGHTS_LIST":     "",
		"NIGHTS_TILL":     "",
		"SORT":            "ASC",
		"TOWNFROMINC":     departure,
		"STATEINC":        countryID,
		"destination":     destination,
		"departure_name":  departure_name,
		"country_name":    countryName,
		"region__name":    regionName,
		"region__name_uz": regionNameUz,
		"region_id":       regionID,
		"region":          "",
		"test":            os.Getenv("TEST"),
	}

	if currentUsdCourse != "" {
		params["current_usd_course"] = currentUsdCourse
	}

	if dateFrom != "" {
		params["CHECKIN_BEG"] = dateFrom
	}
	if dateTo != "" {
		params["CHECKIN_END"] = dateTo
	}
	if town != "" {
		params["TOWNS"] = town
	}
	if minPrice != "" {
		if usdMin, err := s.convertUzsPriceToUsd(minPrice); err != nil {
			logger.Log.Warn().
				Err(err).
				Str("min_price", minPrice).
				Msg("failed to convert min_price to USD for COSTMIN")
		} else {
			params["COSTMIN"] = usdMin
		}
	}
	if maxPrice != "" {
		if usdMax, err := s.convertUzsPriceToUsd(maxPrice); err != nil {
			logger.Log.Warn().
				Err(err).
				Str("max_price", maxPrice).
				Msg("failed to convert max_price to USD for COSTMAX")
		} else {
			params["COSTMAX"] = usdMax
		}
	}
	if hotelRating != "" {
		params["STARS"] = normalizeStarsList(hotelRating)
	}
	if rating != "" {
		params["STARS"] = normalizeStarsList(rating)
	}
	if durationDays != "" {
		params["NIGHTS_LIST"] = durationDays
	}
	if duration != "" {
		params["NIGHTS_LIST"] = duration
	}
	if mealPlan != "" {
		params["MEALS"] = mealPlan
	}
	if meal != "" {
		params["MEALS"] = meal
	}
	if hotelID != "" {
		params["HOTELS"] = hotelID
	}
	if cheapest {
		params["SORT"] = "ASC"
	}
	if mostExpensive {
		params["SORT"] = "DESC"
	}
	if recommendedOnly {
		params["recommended"] = "true"
	} else {
		params["recommended"] = "false"
	}
	params["cheapest"] = strconv.FormatBool(cheapest)
	params["most_expensive"] = strconv.FormatBool(mostExpensive)
	if params["NIGHTS_LIST"] == "" {
		params["NIGHTS_FROM"] = ""
		params["NIGHTS_LIST"] = "7,8,9,10,11,12,13,14"
	}

	return params, false, userSpecifiedDate, nil
}

func (s *SamoService) MapParams(mappedParams map[string]string, operatorName string) (map[string]string, bool, error) {
	stateID, _ := strconv.Atoi(mappedParams["STATEINC"])
	townFromID, _ := strconv.Atoi(mappedParams["TOWNFROMINC"])
	townIDs := splitCSV(mappedParams["TOWNS"])
	DestinationID, _ := strconv.Atoi(mappedParams["destination"])
	mealIDs := splitCSV(mappedParams["MEALS"])
	ratingVals := splitCSV(mappedParams["STARS"])

	if stateID > 0 {
		countryMapping, err := repository.GetCountryMapping(s.DB, operatorName, stateID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No country mapping found for operator: %s, stateID: %d", operatorName, stateID)
				return nil, false, nil
			}
			log.Printf("Error fetching country mapping for operator: %s, stateID: %d, error: %v", operatorName, stateID, err)
			return nil, false, err
		}
		mappedParams["STATEINC"] = strconv.Itoa(countryMapping.OperatorStateID)
		mappedParams["country_name"] = countryMapping.CountryName
		if countryMapping.DestinationImageURL != "" {
			mappedParams["destination_image_url"] = countryMapping.DestinationImageURL
		}
	}

	if townFromID > 0 {
		regionMapping, err := repository.GetRegionMapping(s.DB, operatorName, townFromID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No region mapping found for operator: %s, townFromID: %d", operatorName, townFromID)
				return nil, false, nil
			}
			return nil, false, err
		}
		mappedParams["TOWNFROMINC"] = strconv.Itoa(regionMapping.OperatorTownID)
	}

	if len(townIDs) > 0 {
		operatorTownIDs := make([]string, 0, len(townIDs))
		for _, rawID := range townIDs {
			townID, err := strconv.Atoi(rawID)
			if err != nil || townID <= 0 {
				continue
			}
			townMapping, err := repository.GetTownMapping(s.DB, operatorName, townID)
			if err != nil {
				if err == sql.ErrNoRows {
					log.Printf("No town mapping found for operator: %s, townID: %d", operatorName, townID)
					continue
				}
				return nil, false, err
			}
			operatorTownIDs = append(operatorTownIDs, strconv.Itoa(townMapping.OperatorTownID))
		}
		if len(operatorTownIDs) == 0 {
			log.Printf("No town mappings for operator: %s, towns: %v", operatorName, townIDs)
			return nil, false, nil
		}
		mappedParams["TOWNS"] = strings.Join(operatorTownIDs, ",")
	} else if DestinationID > 0 {
		// Region tanlanganda TOWNS majburiy — bo'sh bo'lsa STATEINC (butun davlat) ga tushmasin
		townMappings, err := repository.GetTownMappingsByRegion(s.DB, operatorName, DestinationID)
		log.Println("townMappings for operator: ", operatorName, " regionID: ", DestinationID, " townMappings: ", townMappings)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("operator", operatorName).
				Int("regionID", DestinationID).
				Msg("error fetching town mappings by region")
			return nil, false, nil
		}
		if len(townMappings) == 0 {
			log.Printf("No town mappings for operator: %s, regionID: %d — skip (no country-wide fallback)", operatorName, DestinationID)
			return nil, false, nil
		}
		operatorTownIDs := make([]string, 0, len(townMappings))
		for _, mapping := range townMappings {
			operatorTownIDs = append(operatorTownIDs, strconv.Itoa(mapping.OperatorTownID))
		}
		mappedParams["TOWNS"] = strings.Join(operatorTownIDs, ",")
	}

	if len(mealIDs) > 0 {
		mealKeys := make([]string, 0, len(mealIDs))
		for _, rawID := range mealIDs {
			mealID, err := strconv.Atoi(rawID)
			if err != nil || mealID <= 0 {
				continue
			}
			mealMapping, err := repository.GetMealPlanMapping(s.DB, operatorName, mealID)
			if err != nil {
				if err == sql.ErrNoRows {
					log.Printf("No meal plan mapping found for operator: %s, mealID: %d", operatorName, mealID)
					continue
				}
				return nil, false, err
			}
			if mealMapping.MealKey != "" {
				mealKeys = append(mealKeys, mealMapping.MealKey)
			}
		}
		if len(mealKeys) == 0 {
			log.Printf("No meal plan mappings for operator: %s, meals: %v", operatorName, mealIDs)
			return nil, false, nil
		}
		mappedParams["MEALS"] = strings.Join(mealKeys, ",")
	}

	if len(ratingVals) > 0 {
		ratingKeys := make([]string, 0, len(ratingVals))
		for _, ratingVal := range ratingVals {
			normalized := normalizeStars(ratingVal)
			if normalized == "" {
				continue
			}
			ratingMapping, err := repository.GetRatingMapping(s.DB, operatorName, normalized)
			if err != nil {
				if err == sql.ErrNoRows {
					log.Printf("No rating mapping found for operator: %s, ratingVal: %s", operatorName, normalized)
					continue
				}
				return nil, false, err
			}
			if ratingMapping.RatingKey != "" {
				ratingKeys = append(ratingKeys, ratingMapping.RatingKey)
			}
		}
		if len(ratingKeys) == 0 {
			log.Printf("No rating mappings for operator: %s, ratings: %v", operatorName, ratingVals)
			return nil, false, nil
		}
		mappedParams["STARS"] = strings.Join(ratingKeys, ",")
	}

	return mappedParams, true, nil
}

func (s *SamoService) ServiceNames() []string {
	names := make([]string, 0)
	seen := make(map[string]struct{})

	for _, cfg := range s.getServiceConfigs() {
		if cfg.Name == "" {
			continue
		}
		if _, ok := seen[cfg.Name]; ok {
			continue
		}
		seen[cfg.Name] = struct{}{}
		names = append(names, cfg.Name)
	}

	return names
}

func (s *SamoService) GetCurrentUsdCourse() (float64, error) {
	return s.currencyService.GetUsdRate(context.Background())
}

func (s *SamoService) convertUzsPriceToUsd(uzsPrice string) (string, error) {
	uzs, err := strconv.ParseFloat(strings.TrimSpace(uzsPrice), 64)
	if err != nil {
		return "", fmt.Errorf("invalid uzs price: %w", err)
	}
	if uzs <= 0 {
		return "", fmt.Errorf("uzs price must be positive")
	}

	rate, err := s.GetCurrentUsdCourse()
	if err != nil {
		return "", fmt.Errorf("failed to get usd rate: %w", err)
	}
	if rate <= 0 {
		return "", fmt.Errorf("invalid usd rate: %f", rate)
	}

	usd := uzs / rate
	return strconv.Itoa(int(math.Round(usd))), nil
}

func (s *SamoService) MakeURLs(params map[string]string) ([]models.Request, error) {
	var urls []models.Request
	configs := s.getServiceConfigs()
	queryOperator := strings.TrimSpace(params["OPERATOR"])

	currentUsdCourse := parsePositiveFloat(params["current_usd_course"], 0)
	if currentUsdCourse <= 0 {
		fetchedRate, err := s.GetCurrentUsdCourse()
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("request bind failed")
			currentUsdCourse = 1.0
		} else {
			currentUsdCourse = fetchedRate
		}
	}

	for _, service := range configs {
		if queryOperator != "" && service.Name != queryOperator {
			continue
		}
		if service.BaseURL == "" || service.OAuthToken == "" {
			continue
		}

		mapped := copyParams(params)
		if service.Name != "" {
			mappedParams, ok, err := s.MapParams(mapped, service.Name)
			if err != nil {
				logger.Log.Error().
					Err(err).
					Str("handler", "async-samo/tickets").
					Str("service", service.Name).
					Msg("error mapping parameters for service")
				return nil, err
			}
			if !ok {
				continue
			}
			mapped = mappedParams
		}

		urlValues := url.Values{}
		queryKeyParams := map[string]string{}
		for k, v := range mapped {
			// if k == "PRICEPAGE" {
			// 	continue
			// }
			if v == "" {
				continue
			}
			urlValues.Set(k, v)
			queryKeyParams[k] = v
		}
		urlValues.Set("oauth_token", service.OAuthToken)

		queryKey := makeQueryKey(queryKeyParams)
		cached, err := repository.GetQueryCache(s.DB, queryKey, service.Name)
		if err != nil && err != sql.ErrNoRows {
			logger.Log.Error().
				Err(err).
				Str("handler", "async-samo/tickets").
				Str("service", service.Name).
				Msg("error fetching query cache")
			// return nil, err
		}

		fullURL := service.BaseURL + "?" + urlValues.Encode()
		fmt.Println("Mapped for ", service, " ", mapped)
		if cached != nil {
			fullURL = cached.URL
		} else {
			if err := repository.SaveQueryCache(s.DB, queryKey, service.Name, fullURL, urlValues.Get("destination_image_url")); err != nil {
				logger.Log.Error().
					Err(err).
					Str("handler", "async-samo/tickets").
					Str("service", service.Name).
					Msg("error saving query cache")
				// log.Printf("Saving query cache for key: %s", queryKey)
				// return nil, err
			}
		}

		destinationID := parsePositiveInt(mapped["destination"])
		departureID := parsePositiveInt(mapped["departure"])
		countryID := parsePositiveInt(params["STATEINC"])
		page := parsePositiveInt(params["PRICEPAGE"])
		if page == 0 {
			page = 1
		}

		urls = append(urls, models.Request{
			Url:              fullURL,
			Operator:         service.Name,
			Departure:        mapped["departure_name"],
			DestinationID:    destinationID,
			DepartureID:      departureID,
			CountryID:        countryID,
			DestCountryName:  mapped["country_name"],
			DestImageUrl:     mapped["destination_image_url"],
			CurrentUsdCourse: currentUsdCourse,
			Istest:           strings.EqualFold(params["test"], "true"),
			Page:             page,
		})
	}
	return urls, nil
}

func (s *SamoService) GetEmptyResults() models.ResultResponse {
	return models.ResultResponse{Prices: []*models.Ticket{}, Total: 0, Page: 1}
}

func parsePositiveInt(value string) int {
	if value == "" {
		return 0
	}
	i, _ := strconv.Atoi(value)
	if i < 0 {
		return 0
	}
	return i
}

func parseFloatOrDefault(value string, def float64) float64 {
	if value == "" {
		return def
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return def
	}
	return f
}

func parsePositiveFloat(value string, def float64) float64 {
	if value == "" {
		return def
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || f <= 0 {
		return def
	}
	return f
}

func formatDate(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "-", "")
}

func normalizeStars(value string) string {
	if value == "" {
		return ""
	}
	trimmed := strings.TrimSpace(value)
	if strings.Contains(trimmed, ".") {
		return trimmed
	}
	if _, err := strconv.Atoi(trimmed); err == nil {
		return trimmed + ".0"
	}
	return trimmed
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeStarsList(value string) string {
	parts := splitCSV(value)
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := normalizeStars(part)
		if normalized != "" {
			out = append(out, normalized)
		}
	}
	return strings.Join(out, ",")
}

func copyParams(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func makeQueryKey(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := url.Values{}
	for _, k := range keys {
		values.Set(k, params[k])
	}
	return values.Encode()
}
