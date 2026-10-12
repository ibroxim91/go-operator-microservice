package turvisorsearch

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go-operator-service/models"
	"go-operator-service/repository"
	"go-operator-service/services"
	"go-operator-service/utils"
)

const (
	operatorName = "turvisor"
	searchURL    = "https://api.tourvisor.ru/search/api/v1/tours/search"
	searchWait   = 10 * time.Second
	pollEvery    = 3 * time.Second
	maxHotels    = 30
	maxDateSpan  = 20 // 21 calendar days, inclusive
	maxNightSpan = 10
)

// Query is a Tourvisor search that does not go through the SAMO URL jobs.
type Query struct {
	DepartureID   int
	CountryID     int
	OurCountryID  int
	RegionIDs     []int
	DateFrom      time.Time
	DateTo        time.Time
	NightsFrom    int
	NightsTo      int
	Adults        int
	Meal          int
	HotelCategory int
	HotelIDs      []int
	PriceFrom     int
	PriceTo       int
	DepartureName string
	CountryName   string
}

// ShouldSearch reports whether this request should call Tourvisor.
// An empty operator searches SAMO and Tourvisor together. A named SAMO operator does not.
func ShouldSearch(operator string) bool {
	operator = strings.TrimSpace(operator)
	return operator == "" || strings.EqualFold(operator, operatorName)
}

// MapParams reads the existing SAMO parameter map and the operator=turvisor rows.
// Missing country, departure, meal, or rating mappings skip this search.
// A selected region with no town mapping also skips it and does not widen to the whole country.
func MapParams(db *sql.DB, params map[string]string) (*Query, bool, error) {
	if db == nil {
		return nil, false, fmt.Errorf("db is nil")
	}
	if params == nil {
		return nil, false, nil
	}

	stateID := atoi(params["STATEINC"])
	townFromID := atoi(params["TOWNFROMINC"])
	if stateID <= 0 || townFromID <= 0 {
		return nil, false, nil
	}

	countryMapping, err := repository.GetCountryMapping(db, operatorName, stateID)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("No country mapping found for operator: %s, stateID: %d", operatorName, stateID)
			return nil, false, nil
		}
		return nil, false, err
	}
	regionMapping, err := repository.GetRegionMapping(db, operatorName, townFromID)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("No region mapping found for operator: %s, townFromID: %d", operatorName, townFromID)
			return nil, false, nil
		}
		return nil, false, err
	}

	destinationID := atoi(params["destination"])
	townIDs := splitCSV(params["TOWNS"])
	regionIDs, ok, err := mapRegionIDs(db, destinationID, townIDs)
	if err != nil || !ok {
		return nil, false, err
	}

	meal, ok, err := mapMeal(db, params["MEALS"])
	if err != nil || !ok {
		return nil, false, err
	}
	hotelCategory, ok, err := mapHotelCategory(db, params["STARS"])
	if err != nil || !ok {
		return nil, false, err
	}

	dateFrom, dateOK := parseYYYYMMDD(params["CHECKIN_BEG"])
	dateTo, dateToOK := parseYYYYMMDD(params["CHECKIN_END"])
	if !dateOK {
		return nil, false, nil
	}
	if !dateToOK {
		dateTo = dateFrom
	}
	dateFrom, dateTo = clampDates(dateFrom, dateTo)
	nightsFrom, nightsTo := nightsWindow(params["NIGHTS_LIST"])

	adults := atoi(params["ADULT"])
	if adults < 1 {
		adults = 1
	}
	if adults > 6 {
		adults = 6
	}

	query := &Query{
		DepartureID:   regionMapping.OperatorTownID,
		CountryID:     countryMapping.OperatorStateID,
		OurCountryID:  stateID,
		RegionIDs:     regionIDs,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
		NightsFrom:    nightsFrom,
		NightsTo:      nightsTo,
		Adults:        adults,
		Meal:          meal,
		HotelCategory: hotelCategory,
		PriceFrom:     atoi(params["COSTMIN"]),
		PriceTo:       atoi(params["COSTMAX"]),
		DepartureName: strings.TrimSpace(params["departure_name"]),
		CountryName:   strings.TrimSpace(countryMapping.CountryName),
	}
	if includeHotelFilter(params["OPERATOR"]) {
		query.HotelIDs = hotelIDs(params["HOTELS"])
	}
	if query.DepartureID <= 0 || query.CountryID <= 0 {
		return nil, false, nil
	}
	return query, true, nil
}

func mapRegionIDs(db *sql.DB, destinationID int, townIDs []string) ([]int, bool, error) {
	if len(townIDs) > 0 {
		ids := make([]int, 0, len(townIDs))
		for _, rawID := range townIDs {
			townID := atoi(rawID)
			if townID <= 0 {
				continue
			}
			mapping, err := repository.GetTownMapping(db, operatorName, townID)
			if err != nil {
				if err == sql.ErrNoRows {
					log.Printf("No town mapping found for operator: %s, townID: %d", operatorName, townID)
					continue
				}
				return nil, false, err
			}
			if mapping.OperatorTownID > 0 {
				ids = append(ids, mapping.OperatorTownID)
			}
		}
		ids, ok := resolveRegionIDs(true, ids)
		return ids, ok, nil
	}
	if destinationID > 0 {
		mappings, err := repository.GetTownMappingsByRegion(db, operatorName, destinationID)
		if err != nil {
			return nil, false, err
		}
		ids := make([]int, 0, len(mappings))
		for _, mapping := range mappings {
			if mapping.OperatorTownID > 0 {
				ids = append(ids, mapping.OperatorTownID)
			}
		}
		ids, ok := resolveRegionIDs(true, ids)
		if !ok {
			log.Printf("No town mappings for operator: %s, regionID: %d — skip (no country-wide fallback)", operatorName, destinationID)
		}
		return ids, ok, nil
	}
	ids, ok := resolveRegionIDs(false, nil)
	return ids, ok, nil
}

// resolveRegionIDs keeps the whole-country search separate from a selected region.
// A selected region or town list with no mapping is skipped. No selection searches the whole country.
func resolveRegionIDs(regionSelected bool, mapped []int) ([]int, bool) {
	if !regionSelected {
		return nil, true
	}
	if len(mapped) == 0 {
		return nil, false
	}
	return mapped, true
}

func mapMeal(db *sql.DB, raw string) (int, bool, error) {
	mealIDs := splitCSV(raw)
	if len(mealIDs) == 0 {
		return 0, true, nil
	}
	keys := make([]string, 0, len(mealIDs))
	for _, rawID := range mealIDs {
		mealID := atoi(rawID)
		if mealID <= 0 {
			continue
		}
		mapping, err := repository.GetMealPlanMapping(db, operatorName, mealID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No meal plan mapping found for operator: %s, mealID: %d", operatorName, mealID)
				continue
			}
			return 0, false, err
		}
		if strings.TrimSpace(mapping.MealKey) != "" {
			keys = append(keys, mapping.MealKey)
		}
	}
	if len(keys) == 0 {
		log.Printf("No meal plan mappings for operator: %s, meals: %v", operatorName, mealIDs)
		return 0, false, nil
	}
	meal := minimumMeal(keys)
	if meal <= 0 {
		return 0, false, nil
	}
	return meal, true, nil
}

func mapHotelCategory(db *sql.DB, raw string) (int, bool, error) {
	ratings := splitCSV(raw)
	if len(ratings) == 0 {
		return 0, true, nil
	}
	keys := make([]string, 0, len(ratings))
	for _, rating := range ratings {
		normalized := normalizeStars(rating)
		if normalized == "" {
			continue
		}
		mapping, err := repository.GetRatingMapping(db, operatorName, normalized)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No rating mapping found for operator: %s, ratingVal: %s", operatorName, normalized)
				continue
			}
			return 0, false, err
		}
		if strings.TrimSpace(mapping.RatingKey) != "" {
			keys = append(keys, mapping.RatingKey)
		}
	}
	if len(keys) == 0 {
		log.Printf("No rating mappings for operator: %s, ratings: %v", operatorName, ratings)
		return 0, false, nil
	}
	category := minimumHotelCategory(keys)
	if category <= 0 {
		return 0, false, nil
	}
	return category, true, nil
}

func includeHotelFilter(operator string) bool {
	return strings.TrimSpace(operator) != ""
}

func hotelIDs(raw string) []int {
	parts := splitCSV(raw)
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id := atoi(part)
		if id <= 0 {
			continue
		}
		ids = append(ids, id)
		if len(ids) == maxHotels {
			break
		}
	}
	return ids
}

// Values builds the tour-search query. The token is never placed in the URL.
func (q Query) Values() url.Values {
	values := url.Values{}
	values.Set("departureId", strconv.Itoa(q.DepartureID))
	values.Set("countryId", strconv.Itoa(q.CountryID))
	values.Set("dateFrom", q.DateFrom.Format("2006-01-02"))
	values.Set("dateTo", q.DateTo.Format("2006-01-02"))
	values.Set("nightsFrom", strconv.Itoa(q.NightsFrom))
	values.Set("nightsTo", strconv.Itoa(q.NightsTo))
	values.Set("adults", strconv.Itoa(q.Adults))
	values.Set("currency", "CU")
	values.Set("onlyCharter", "true")
	if q.Meal > 0 {
		values.Set("meal", strconv.Itoa(q.Meal))
	}
	if q.HotelCategory > 0 {
		values.Set("hotelCategory", strconv.Itoa(q.HotelCategory))
	}
	if q.PriceFrom > 0 {
		values.Set("priceFrom", strconv.Itoa(q.PriceFrom))
	}
	if q.PriceTo > 0 {
		values.Set("priceTo", strconv.Itoa(q.PriceTo))
	}
	if len(q.RegionIDs) > 0 {
		values.Set("regionIds", joinInts(q.RegionIDs))
	}
	if len(q.HotelIDs) > 0 {
		values.Set("hotelIds", joinInts(q.HotelIDs))
	}
	return values
}

// MakeURL is the search start URL for the mapped query.
func MakeURL(q Query) string {
	return searchURL + "?" + q.Values().Encode()
}

// Search starts a Tourvisor search, waits up to 30 seconds, and maps the tours that arrived.
func Search(ctx context.Context, db *sql.DB, params map[string]string, usdCourse float64) ([]*models.Ticket, error) {
	query, ok, err := MapParams(db, params)
	if err != nil || !ok {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	searchID, err := startSearch(ctx, query)
	if err != nil {
		return nil, err
	}
	waitForStatus(ctx, searchID)
	hotels, err := fetchResults(ctx, searchID)
	if err != nil {
		return nil, err
	}
	tickets := make([]*models.Ticket, 0)
	for _, hotel := range hotels {
		for _, tour := range hotel.Tours {
			ticket := mapSearchTour(hotel, tour, *query, usdCourse)
			if ticket != nil {
				tickets = append(tickets, ticket)
			}
		}
	}
	return tickets, nil
}

func startSearch(ctx context.Context, query *Query) (string, error) {
	body, status, err := getJSON(ctx, MakeURL(*query))
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("tourvisor search status %d", status)
	}
	searchID, err := decodeSearchID(body)
	if err != nil {
		return "", err
	}
	log.Printf("tourvisor search started id=%s", searchID)
	return searchID, nil
}

func waitForStatus(ctx context.Context, searchID string) {
	deadline := time.Now().Add(searchWait)
	for {
		body, status, err := getJSON(ctx, statusURL(searchID))
		if err == nil && status >= 200 && status < 300 {
			if statusDone(decodeStatus(body)) {
				return
			}
		}
		if time.Now().After(deadline) || time.Until(deadline) < 500*time.Millisecond {
			return
		}
		wait := pollEvery
		if remaining := time.Until(deadline); remaining < wait {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func fetchResults(ctx context.Context, searchID string) ([]searchHotel, error) {
	endpoint := resultsURL(searchID)
	body, status, err := getJSON(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("tourvisor results status %d", status)
	}
	return decodeResults(body)
}

func getJSON(ctx context.Context, endpoint string) ([]byte, int, error) {
	token := strings.TrimSpace(os.Getenv("TOURVISOR_JWT"))
	if token == "" {
		return nil, 0, fmt.Errorf("TOURVISOR_JWT is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func statusURL(searchID string) string {
	return searchURL + "/" + url.PathEscape(searchID) + "/status"
}

func resultsURL(searchID string) string {
	return searchURL + "/" + url.PathEscape(searchID) + "?limit=100"
}

type flexID string

func (f *flexID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*f = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*f = flexID(value)
		return nil
	}
	*f = flexID(data)
	return nil
}

type searchHotel struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Category    int     `json:"category"`
	Rating      float64 `json:"rating"`
	Picturelink string  `json:"picturelink"`
	Currency    string  `json:"currency"`
	Country     struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"country"`
	Region struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"region"`
	Tours []searchTour `json:"tours"`
}

type searchTour struct {
	ID       flexID  `json:"id"`
	TourID   flexID  `json:"tourId"`
	Date     string  `json:"date"`
	Nights   int     `json:"nights"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	Adults   int     `json:"adults"`
	Childs   int     `json:"childs"`
	RoomType string  `json:"roomType"`
	Meal     struct {
		Name     string `json:"name"`
		FullName string `json:"fullName"`
	} `json:"meal"`
}

func (t searchTour) tourID() string {
	if strings.TrimSpace(string(t.ID)) != "" {
		return strings.TrimSpace(string(t.ID))
	}
	return strings.TrimSpace(string(t.TourID))
}

func decodeSearchID(body []byte) (string, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("empty tourvisor search id")
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		id := strings.Trim(string(trimmed), `"`)
		if id == "" || id == "0" || id == "null" {
			return "", fmt.Errorf("empty tourvisor search id")
		}
		return id, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return "", err
	}
	if raw, ok := payload["data"]; ok {
		if id, err := decodeSearchID(raw); err == nil {
			return id, nil
		}
	}
	for _, key := range []string{"searchId", "search_id", "id"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		id := strings.Trim(string(bytes.TrimSpace(raw)), `"`)
		if id != "" && id != "0" && id != "null" {
			return id, nil
		}
	}
	return "", fmt.Errorf("tourvisor search id missing")
}

type searchStatus struct {
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
}

func decodeStatus(body []byte) searchStatus {
	trimmed := bytes.TrimSpace(body)
	var wrapped struct {
		Data     json.RawMessage `json:"data"`
		Progress float64         `json:"progress"`
		Status   string          `json:"status"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return searchStatus{}
	}
	if len(bytes.TrimSpace(wrapped.Data)) > 0 && bytes.TrimSpace(wrapped.Data)[0] == '{' {
		var inner searchStatus
		if err := json.Unmarshal(wrapped.Data, &inner); err == nil && (inner.Progress > 0 || inner.Status != "") {
			return inner
		}
	}
	return searchStatus{Progress: wrapped.Progress, Status: wrapped.Status}
}

func statusDone(status searchStatus) bool {
	if status.Progress >= 100 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status.Status)) {
	case "finished", "complete", "completed", "done", "ready", "success":
		return true
	default:
		return false
	}
}

func decodeResults(body []byte) ([]searchHotel, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var hotels []searchHotel
		if err := json.Unmarshal(trimmed, &hotels); err != nil {
			return nil, err
		}
		return hotels, nil
	}
	var wrapped struct {
		Data   json.RawMessage `json:"data"`
		Hotels []searchHotel   `json:"hotels"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return nil, err
	}
	if len(wrapped.Hotels) > 0 {
		return wrapped.Hotels, nil
	}
	data := bytes.TrimSpace(wrapped.Data)
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	if data[0] == '[' {
		var hotels []searchHotel
		if err := json.Unmarshal(data, &hotels); err != nil {
			return nil, err
		}
		return hotels, nil
	}
	var nested struct {
		Hotels []searchHotel   `json:"hotels"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &nested); err != nil {
		return nil, err
	}
	if len(nested.Hotels) > 0 {
		return nested.Hotels, nil
	}
	inner := bytes.TrimSpace(nested.Data)
	if len(inner) > 0 && inner[0] == '[' {
		var hotels []searchHotel
		if err := json.Unmarshal(inner, &hotels); err != nil {
			return nil, err
		}
		return hotels, nil
	}
	return nil, nil
}

func mapSearchTour(hotel searchHotel, tour searchTour, query Query, usdCourse float64) *models.Ticket {
	tourID := tour.tourID()
	if tourID == "" || hotel.ID <= 0 {
		return nil
	}
	priceUZS := 0
	if usdCourse > 0 {
		priceUZS = utils.ConvertOperatorPriceToUzs(tour.Price, usdCourse)
	}
	nights := tour.Nights
	if nights < 0 {
		nights = 0
	}
	checkIn := normalizeDate(tour.Date)
	checkOut := addNights(checkIn, nights)
	meal := strings.TrimSpace(tour.Meal.Name)
	if meal == "" {
		meal = strings.TrimSpace(tour.Meal.FullName)
	}
	if meal == "" {
		meal = "RO"
	}
	rating := hotel.Category
	if rating == 0 && hotel.Rating > 0 {
		rating = int(math.Round(hotel.Rating))
	}
	photo := hotelPhotoURL(hotel.Picturelink)
	countryName := strings.TrimSpace(hotel.Country.Name)
	if countryName == "" {
		countryName = query.CountryName
	}
	regionName := strings.TrimSpace(hotel.Region.Name)
	departureName := query.DepartureName
	if departureName == "" {
		departureName = "Ташкент"
	}
	adults := tour.Adults
	if adults <= 0 {
		adults = query.Adults
	}
	if adults <= 0 {
		adults = 2
	}
	children := tour.Childs
	if children < 0 {
		children = 0
	}
	countryID := hotel.Country.ID
	if countryID == 0 {
		countryID = query.CountryID
	}
	return &models.Ticket{
		Title:           hotel.Name,
		Slug:            tourSlug(hotel.Name, tourID),
		DepartureDate:   checkIn,
		DepartureTime:   checkIn,
		TravelTime:      checkOut,
		Nights:          nights,
		DurationDays:    nights,
		PassengerCount:  adults + children,
		Price:           formatPriceMillion(priceUZS),
		PriceFull:       priceUZS,
		OperatorPrice:   strconv.FormatFloat(tour.Price, 'f', -1, 64),
		Currency:        "UZS",
		Operator:        operatorName,
		TourOperatorID:  tourID,
		CountryID:       query.OurCountryID,
		DestinationID:   hotel.Region.ID,
		HotelPhoto:      photo,
		HotelPhotoCount: boolToCount(photo != ""),
		HotelPhotos:     hotelPhotos(photo),
		TicketImages:    photo,
		Rating:          hotel.Rating,
		RoomType:        strings.TrimSpace(tour.RoomType),
		VisaRequired:    services.IsCountryVisaRequired(query.OurCountryID),
		Departure: models.DepartureInfo{
			ID:      query.DepartureID,
			Name:    departureName,
			Country: "Uzbekistan",
		},
		Destination: models.DestinationInfo{
			ID:   hotel.Region.ID,
			Name: regionName,
			Country: models.CountryInfo{
				ID:   countryID,
				Name: countryName,
			},
		},
		TicketAmenities:  []string{},
		Transports:       []models.Transport{},
		Tariff:           []models.Tariff{},
		ExtraService:     []string{},
		PaidExtraService: []string{},
		TicketHotel: []models.TicketHotel{{
			ID:       hotel.ID,
			Name:     hotel.Name,
			Rating:   rating,
			MealPlan: meal,
		}},
	}
}

func clampDates(from, to time.Time) (time.Time, time.Time) {
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if to.Before(from) {
		to = from
	}
	if to.After(from.AddDate(0, 0, maxDateSpan)) {
		to = from.AddDate(0, 0, maxDateSpan)
	}
	return from, to
}

func nightsWindow(list string) (int, int) {
	parts := splitCSV(list)
	if len(parts) == 0 {
		return 7, 14
	}
	minN := 0
	maxN := 0
	for _, part := range parts {
		n := atoi(part)
		if n <= 0 {
			continue
		}
		if minN == 0 || n < minN {
			minN = n
		}
		if n > maxN {
			maxN = n
		}
	}
	if minN == 0 {
		return 7, 14
	}
	if minN > 28 {
		minN = 28
	}
	if maxN < minN {
		maxN = minN
	}
	if maxN > 28 {
		maxN = 28
	}
	if maxN-minN > maxNightSpan {
		maxN = minN + maxNightSpan
	}
	if maxN > 28 {
		maxN = 28
	}
	return minN, maxN
}

func minimumMeal(keys []string) int {
	minMeal := 0
	for _, key := range keys {
		n := atoi(strings.TrimSpace(key))
		if n <= 0 {
			continue
		}
		if minMeal == 0 || n < minMeal {
			minMeal = n
		}
	}
	return minMeal
}

func minimumHotelCategory(keys []string) int {
	minCategory := 0
	for _, key := range keys {
		n := parseCategory(key)
		if n <= 0 {
			continue
		}
		if minCategory == 0 || n < minCategory {
			minCategory = n
		}
	}
	return minCategory
}

func parseCategory(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return clampCategory(n)
	}
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return clampCategory(int(math.Round(n)))
	}
	digits := strings.Builder{}
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	n, err := strconv.Atoi(digits.String())
	if err != nil {
		return 0
	}
	return clampCategory(n)
}

func clampCategory(n int) int {
	if n < 1 || n > 5 {
		return 0
	}
	return n
}

func normalizeDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, layout := range []string{"20060102", "2006-01-02", "02.01.2006"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed.Format("20060102")
		}
	}
	return raw
}

func addNights(checkIn string, nights int) string {
	if nights < 0 {
		nights = 0
	}
	parsed, err := time.Parse("20060102", checkIn)
	if err != nil {
		return ""
	}
	return parsed.AddDate(0, 0, nights).Format("20060102")
}

func hotelPhotoURL(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "//") {
		return "https:" + link
	}
	return link
}

func hotelPhotos(photo string) []models.HotelPhotos {
	if photo == "" {
		return nil
	}
	return []models.HotelPhotos{{Image: photo}}
}

func boolToCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

func formatPriceMillion(priceValueUsz int) string {
	if priceValueUsz <= 0 {
		return "0"
	}
	mln := float64(priceValueUsz) / 1_000_000
	if math.Mod(mln, 1) == 0 {
		return fmt.Sprintf("%d", int(mln))
	}
	return fmt.Sprintf("%.1f", mln)
}

func tourSlug(name, tourID string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "'", "", "\"", "")
	base = replacer.Replace(base)
	if base == "" {
		base = "tour"
	}
	if tourID != "" {
		return base + "-" + tourID
	}
	return base
}

func parseYYYYMMDD(value string) (time.Time, bool) {
	parsed, err := time.Parse("20060102", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func atoi(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return n
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
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

func joinInts(ids []int) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		parts = append(parts, strconv.Itoa(id))
	}
	return strings.Join(parts, ",")
}

func normalizeStars(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, ".") {
		return trimmed
	}
	if _, err := strconv.Atoi(trimmed); err == nil {
		return trimmed + ".0"
	}
	return trimmed
}
