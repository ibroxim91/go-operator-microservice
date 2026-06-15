package services

import (
	"database/sql"
	"go-operator-service/cache"

	"strings"
	"sync"

	"github.com/xrash/smetrics"
)



type HotelPhoto struct {
	URL   string
	Note  string
	Count int // qo‘shimcha field
}

var (
	dbQueryCounter int
	hotelsCache    map[int][]cache.Hotel
	cacheErr       error
	cacheH        sync.RWMutex
	photoCache     = map[int]*HotelPhoto{}
	photoMu        sync.RWMutex
	cacheMu        sync.RWMutex
	
	mappingOnce sync.Once
	cacheOnce      sync.Once
    mappingErr error
)



func GetHotelData(
	db *sql.DB,
	hotelID int,
	hotelName string,
	operator string,
	countryID int,
	mealPlan string,
) (*cache.Hotel, bool, error) {

	if err := PreloadHotelsCache(db); err != nil {
		return nil, false, err
	}

	if err := PreloadHotelMappings(db); err != nil {
		return nil, false, err
	}

	// 1. mappingdan qidir
	// hotel, err := FindHotelByMapping(
	// 	operator,
	// 	hotelID,
	// )

	// if err == nil {
	// 	return hotel, true, nil
	// }

	// 2. fallback similarity
	hotel, err := FindHotelByName(
		countryID,
		hotelName,
	)

	if err != nil {
		return nil, false, err
	}

	// 3. mapping saqlash (background worker queue orqali)
	EnqueueHotelMapping(HotelMappingJob{
		Operator:        operator,
		OperatorHotelID: hotelID,
		HotelID:         hotel.ID,
	})

	return hotel, false, nil
}



func FindHotelByMapping(
	operator string,
	operatorHotelID int,
) (*cache.Hotel, error) {

	hotelID, ok := cache.GetMappedHotelID(operator, operatorHotelID)

	if !ok {
		return nil, sql.ErrNoRows
	}

	cacheH.RLock()
	hotel, ok := cache.HotelByIDCache[hotelID]
	cacheH.RUnlock()

	if !ok {
		return nil, sql.ErrNoRows
	}

	return &hotel, nil
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

func similarity(a, b string) float64 {

	aWords := strings.Fields(a)
	bWords := strings.Fields(b)

	matchCount := 0

	for _, aw := range aWords {
		for _, bw := range bWords {

			if aw == bw {
				matchCount++
				break
			}
		}
	}

	maxWords := len(aWords)
	if len(bWords) > maxWords {
		maxWords = len(bWords)
	}

	if maxWords == 0 {
		return 0
	}

	return (float64(matchCount) / float64(maxWords)) * 100
}

func FindHotelByName(countryID int, hotelName string) (*cache.Hotel, error) {

	cacheH.RLock()
	hotels, ok := hotelsCache[countryID]
	cacheH.RUnlock()

	if !ok {
		return nil, sql.ErrNoRows
	}

	target := normalize(hotelName)

	var bestHotel *cache.Hotel
	bestScore := 0.0

	for i := range hotels {

		hotel := &hotels[i]
		hotelNormalized := normalize(hotel.Name)

		score := combinedScore(
			target,
			hotelNormalized,
		)

		if score > bestScore {
			bestScore = score
			bestHotel = hotel
		}
	}

	// threshold
	if bestHotel != nil && bestScore >= 70 {
		// log.Println(
		// 	"Hotel  found in cache for name:",
		// 	hotelName, " Db hotel ", bestHotel.Name,
		// 	"| COUNTRYID:",
		// 	countryID,
		// 	"| bestScore:",
		// 	bestScore,
		// )
		return bestHotel, nil
	}

	return nil, sql.ErrNoRows
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
		for rows.Next() {
			var h cache.Hotel
			var countryID int
			if err := rows.Scan(&h.ID, &h.Name, &countryID); err != nil {
				continue
			}
			cache.HotelByIDCache[h.ID] = h
			tempCache[countryID] = append(tempCache[countryID], h)
		}
		if err := rows.Err(); err != nil {
			cacheErr = err
			return
		}

		cacheMu.Lock()
		hotelsCache = tempCache
		cacheMu.Unlock()
	})

	return cacheErr
}