package services

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/repository"

	"github.com/labstack/echo/v4"
)

type SamoService struct {
	DB *sql.DB
}

type SamoServiceConfig struct {
	Name       string
	BaseURL    string
	OAuthToken string
}

func NewSamoService(db *sql.DB) *SamoService {
	return &SamoService{DB: db}
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
	}
	return services

}

func (s *SamoService) GetSamoParams(c echo.Context) (map[string]string, bool, error) {
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
	_ = formatDate(getTrimmed("dateFrom", ""))
	_ = formatDate(getTrimmed("dateTo", ""))
	adults := getTrimmed("adults", "1")
	children := getTrimmed("children", "0")
	operator := getTrimmed("operator", "")
	departure := getTrimmed("departure", "")
	destination := getTrimmed("destination", "")
	countryName := ""
	regionName := ""
	regionNameUz := ""
	regionID := ""
	minDepartureDate := formatDate(getTrimmed("min_departure_date", ""))
	maxDepartureDate := formatDate(getTrimmed("max_departure_date", ""))
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
				return nil, false, err
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
	log.Println("CountryId ", countryID, " | Destination ", destination, " | Town ", town, " | Departure ", departure, " | From cache ", fromCache)

	if departure == "" || countryID == "" {

		return map[string]string{}, true, nil

		// return nil, false, errors.New("missing required departure or country_id")
	}

	today := time.Now()
	checkinBeg := today.Add(3 * 24 * time.Hour).Format("20060102")
	checkinEnd := today.Add(20 * 24 * time.Hour).Format("20060102")

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
		"NIGHTS_FROM":     "3",
		"NIGHTS_LIST":     "",
		"NIGHTS_TILL":     "14",
		"SORT":            "ASC",
		"TOWNFROMINC":     departure,
		"STATEINC":        countryID,
		"destination":     destination,
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

	if minDepartureDate != "" {
		params["CHECKIN_BEG"] = minDepartureDate
	}
	if maxDepartureDate != "" {
		params["CHECKIN_END"] = maxDepartureDate
	}
	if town != "" {
		params["TOWNS"] = town
	}
	if minPrice != "" {
		params["COSTMIN"] = minPrice
	}
	if maxPrice != "" {
		params["COSTMAX"] = maxPrice
	}
	if hotelRating != "" {
		params["STARS"] = normalizeStars(hotelRating)
	}
	if rating != "" {
		params["STARS"] = normalizeStars(rating)
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
	if params["NIGHTS_LIST"] == "" {
		params["NIGHTS_LIST"] = "2,3,4,5,6,7"
	}

	return params, false, nil
}

func (s *SamoService) MapParams(mappedParams map[string]string, operatorName string) (map[string]string, bool, error) {
	stateID, _ := strconv.Atoi(mappedParams["STATEINC"])
	townFromID, _ := strconv.Atoi(mappedParams["TOWNFROMINC"])
	townID, _ := strconv.Atoi(mappedParams["TOWNS"])
	DestinationID, _ := strconv.Atoi(mappedParams["destination"])
	mealID, _ := strconv.Atoi(mappedParams["MEALS"])
	ratingVal := mappedParams["STARS"]

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

	if townID > 0 {
		townMapping, err := repository.GetTownMapping(s.DB, operatorName, townID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No town mapping found for operator: %s, townID: %d", operatorName, townID)
				return nil, false, nil
			}
			return nil, false, err
		}
		mappedParams["TOWNS"] = strconv.Itoa(townMapping.OperatorTownID)
	} else if townID == 0 && townFromID > 0 {
		townMappings, err := repository.GetTownMappingsByRegion(s.DB, operatorName, DestinationID)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("operator", operatorName).
				Int("regionID", DestinationID).
				Msg("error fetching town mappings by region")
			return nil, false, nil
		}
		log.Println()
		log.Println("townMappings for operator ", operatorName, " = ", len(townMappings))
		log.Println()
		if len(townMappings) > 0 {
			operatorTownIDs := make([]string, 0, len(townMappings))
			for _, mapping := range townMappings {
				operatorTownIDs = append(operatorTownIDs, strconv.Itoa(mapping.OperatorTownID))
			}
			mappedParams["TOWNS"] = strings.Join(operatorTownIDs, ",")
		}
	}

	if mealID > 0 {
		mealMapping, err := repository.GetMealPlanMapping(s.DB, operatorName, mealID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No meal plan mapping found for operator: %s, mealID: %d", operatorName, mealID)
				return nil, false, nil
			}
			return nil, false, err
		}
		mappedParams["MEALS"] = mealMapping.MealKey
	}

	if ratingVal != "" {
		ratingMapping, err := repository.GetRatingMapping(s.DB, operatorName, ratingVal)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No rating mapping found for operator: %s, ratingVal: %s", operatorName, ratingVal)
				return nil, false, nil
			}
			return nil, false, err
		}
		mappedParams["STARS"] = ratingMapping.RatingKey
	}

	return mappedParams, true, nil
}

func (s *SamoService) GetCurrentUsdCourse() (float64, error) {
	return CurrencyService{}.GetUsdRate()
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
			Departure:        mapped["departure"],
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
