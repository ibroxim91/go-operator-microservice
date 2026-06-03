package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
)

type Hotel struct {
	ID   int
	Name string
}

type HotelPhoto struct {
	URL   string
	Note  string
	Count int // qo‘shimcha field
}

var (
	dbQueryCounter int
	hotelsCache    map[int][]Hotel
	cacheOnce      sync.Once
	cacheErr       error
	cacheMu        sync.RWMutex
	photoCache     = map[int]*HotelPhoto{}
	photoMu        sync.RWMutex
)

func GetHotelData(db *sql.DB, hotelID int, hotelName, operator string, countryID int, mealPlan string) (*Hotel, bool, error) {
	// Avval barcha hotellar cache ga yuklanadi, so‘ng hotel nomi bo‘yicha qidiriladi
	
	if err := preloadHotelsCache(db); err != nil {
		log.Println("Error loding hotels cache ", err)
		return nil, false, err
	}

	hotel, err := FindHotelByName(countryID, hotelName)
	if err != nil {
		return nil, false, err
	}
	return hotel, false, nil
}

func preloadHotelsCache(db *sql.DB) error {
	cacheOnce.Do(func() {
		query := `SELECT id, name, country_id FROM api_hotel`
		dbQueryCounter++
		rows, err := db.Query(query)
		if err != nil {
			cacheErr = err
			return
		}
		defer rows.Close()

		tempCache := make(map[int][]Hotel)
		for rows.Next() {
			var h Hotel
			var countryID int
			if err := rows.Scan(&h.ID, &h.Name, &countryID); err != nil {
				continue
			}
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

func GetHotelPhoto(db *sql.DB, hotelID int) (*HotelPhoto, error) {

	photoMu.RLock()
	photo, ok := photoCache[hotelID]
	photoMu.RUnlock()

	if ok {
		return photo, nil
	}

	query := `
	SELECT 
		url,
		note,
		COUNT(*) OVER() as total_count
	FROM api_hotelphoto
	WHERE hotel_id = $1
	ORDER BY id ASC
	LIMIT 1
	`

	row := db.QueryRow(query, hotelID)

	dbQueryCounter++

	var p HotelPhoto

	err := row.Scan(
		&p.URL,
		&p.Note,
		&p.Count,
	)

	if err != nil {
		return nil, err
	}

	photoMu.Lock()
	photoCache[hotelID] = &p
	photoMu.Unlock()

	return &p, nil
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
		"residence":  true,
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
func FindHotelByName(countryID int, hotelName string) (*Hotel, error) {

	cacheMu.RLock()
	hotels, ok := hotelsCache[countryID]
	cacheMu.RUnlock()

	// log.Println(
	// 	"FindHotelByName start hotelName:",
	// 	hotelName,
	// 	"| CountryID:",
	// 	countryID,
	// 	"| Hotels:",
	// 	len(hotels),
	// 	"| Cache hit:",
	// 	ok,
	// )

	if !ok {
		return nil, sql.ErrNoRows
	}

	target := normalize(hotelName)

	var bestHotel *Hotel
	bestScore := 0.0

	for _, hotel := range hotels {

		hotelNormalized := normalize(hotel.Name)

		// Contains tekshirish
		if strings.Contains(target, hotelNormalized) ||
			strings.Contains(hotelNormalized, target) {

			fmt.Printf("FOUND BY CONTAINS: %s\n", hotel.Name)
			return &hotel, nil
		}

		score := similarity(target, hotelNormalized)

		fmt.Printf(
			"%-30s -> %.2f%%\n",
			hotel.Name,
			score,
		)

		if score > bestScore {
			bestScore = score
			bestHotel = &hotel
		}
	}

	// log.Println(
	// 	"BEST MATCH:",
	// 	bestHotel.Name,
	// 	"| score:",
	// 	bestScore,
	// )

	// threshold
	if bestHotel != nil && bestScore >= 70 {
		return bestHotel, nil
	}

	// log.Println(
	// 	"Hotel not found in cache for name:",
	// 	hotelName,
	// 	"| CountryID:",
	// 	countryID,
	// )

	return nil, sql.ErrNoRows
}
