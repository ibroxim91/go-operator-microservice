package services

import (
	"database/sql"
	"go-operator-service/cache"
	"go-operator-service/logger"

	"strings"
	"sync"

	"github.com/xrash/smetrics"
)

var (
	dbQueryCounter int
	hotelsCache    map[int][]cache.Hotel
	cacheErr       error
	cacheMu        sync.RWMutex
	mappingOnce    sync.Once
	cacheOnce      sync.Once
	mappingErr     error
)

func FindHotelByMapping(operator string, operatorHotelID int) (*cache.Hotel, error) {
	hotelID, ok := cache.GetMappedHotelID(operator, operatorHotelID)
	if !ok {
		return nil, sql.ErrNoRows
	}

	cacheMu.RLock()
	hotel, ok := cache.HotelByIDCache[hotelID]
	cacheMu.RUnlock()

	if !ok {
		return nil, sql.ErrNoRows
	}

	return &hotel, nil
}

func FindHotelByNameWithScore(countryID int, hotelName string) (*cache.Hotel, float64, error) {
	cacheMu.RLock()
	hotels, ok := hotelsCache[countryID]
	cacheMu.RUnlock()

	if !ok {
		return nil, 0, sql.ErrNoRows
	}

	target := normalize(hotelName)

	var bestHotel *cache.Hotel
	bestScore := 0.0

	for i := range hotels {
		hotel := &hotels[i]
		score := combinedScore(target, normalize(hotel.Name))
		if score > bestScore {
			bestScore = score
			bestHotel = hotel
		}
	}

	logger.Log.Debug().
		Str("hotel_name", hotelName).
		Int("country_id", countryID).
		Float64("similarity_score", bestScore).
		Msg("hotel name similarity evaluated")

	if bestHotel != nil && bestScore >= hotelMatchScoreThreshold {
		logger.Log.Info().
			Str("hotel_name", hotelName).
			Str("matched_hotel_name", bestHotel.Name).
			Int("matched_hotel_id", bestHotel.ID).
			Int("country_id", countryID).
			Float64("similarity_score", bestScore).
			Msg("hotel matched by name fallback")
		return bestHotel, bestScore, nil
	}

	return nil, bestScore, sql.ErrNoRows
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	replacer := strings.NewReplacer(
		"&", "and",
		"-", " ",
		"_", " ",
		".", "",
		",", "",
		"*", "",
	)

	s = replacer.Replace(s)

	stopWords := map[string]bool{
		"hotel":      true,
		"resort":     true,
		"spa":        true,
		"apartments": true,
		"apartment":  true,
		"baku":       true,
		"баку":       true,
	}

	words := strings.Fields(s)

	var filtered []string

	for _, w := range words {
		if !stopWords[w] {
			filtered = append(filtered, w)
		}
	}

	return strings.Join(filtered, " ")
}

func combinedScore(a, b string) float64 {
	token := tokenSimilarity(a, b)
	jaro := jaroScore(a, b)

	return token*0.7 + jaro*0.3
}

func tokenSimilarity(a, b string) float64 {
	aWords := strings.Fields(a)
	bWords := strings.Fields(b)

	if len(aWords) == 0 || len(bWords) == 0 {
		return 0
	}

	bSet := make(map[string]struct{})

	for _, word := range bWords {
		bSet[word] = struct{}{}
	}

	matches := 0

	for _, word := range aWords {
		if _, ok := bSet[word]; ok {
			matches++
		}
	}

	maxWords := len(aWords)

	if len(bWords) > maxWords {
		maxWords = len(bWords)
	}

	return (float64(matches) / float64(maxWords)) * 100
}

func jaroScore(a, b string) float64 {
	return smetrics.JaroWinkler(
		a,
		b,
		0.7,
		4,
	) * 100
}

func PreloadHotelMappings(db *sql.DB) error {
	mappingOnce.Do(func() {
		query := `
        SELECT
            operator,
            operator_hotel_id,
            hotel_id
        FROM hotel_mapping
        `
		rows, err := db.Query(query)
		if err != nil {
			mappingErr = err
			return
		}
		defer rows.Close()

		tmp := make(map[string]int)
		for rows.Next() {
			var operator string
			var operatorHotelID int
			var hotelID int

			if err := rows.Scan(&operator, &operatorHotelID, &hotelID); err != nil {
				continue
			}
			key := cache.HotelMappingKey(operator, operatorHotelID)
			tmp[key] = hotelID
		}

		cache.MappingCacheMu.Lock()
		cache.HotelMappingCache = tmp
		cache.MappingCacheMu.Unlock()
	})

	return mappingErr
}

func PreloadHotelsCache(db *sql.DB) error {
	cacheOnce.Do(func() {
		query := `SELECT id, name, country_id FROM api_hotel`
		dbQueryCounter++
		rows, err := db.Query(query)
		if err != nil {
			cacheErr = err
			return
		}
		defer rows.Close()

		tempCache := make(map[int][]cache.Hotel)
		tempByID := make(map[int]cache.Hotel)
		for rows.Next() {
			var h cache.Hotel
			var countryID int
			if err := rows.Scan(&h.ID, &h.Name, &countryID); err != nil {
				continue
			}
			tempByID[h.ID] = h
			tempCache[countryID] = append(tempCache[countryID], h)
		}
		if err := rows.Err(); err != nil {
			cacheErr = err
			return
		}

		cacheMu.Lock()
		hotelsCache = tempCache
		cache.HotelByIDCache = tempByID
		cacheMu.Unlock()
	})

	return cacheErr
}
