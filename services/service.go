package services

import (
	"database/sql"
	"fmt"
	"sync"

	"go-operator-service/cache"
	"go-operator-service/models"
)

const hotelMatchScoreThreshold = 70.0

type HotelService struct {
	DB           *sql.DB
	requestCache sync.Map
}

type HotelWithPhoto struct {
	ID          int
	Name        string
	Photo       string
	Count       int
	HotelPhotos []models.HotelPhotos
	FromMapping bool
}

type cachedHotelResult struct {
	hotel       *HotelWithPhoto
	fromMapping bool
}

// NewHotelService constructor
func NewHotelService(db *sql.DB) *HotelService {
	return &HotelService{DB: db}
}

// BeginSearch clears per-request hotel resolution cache.
func (s *HotelService) BeginSearch() {
	s.requestCache = sync.Map{}
}

// EndSearch drops per-request hotel resolution cache.
func (s *HotelService) EndSearch() {
	s.requestCache = sync.Map{}
}

func requestHotelCacheKey(operator string, operatorHotelID int) string {
	return cache.HotelMappingKey(operator, operatorHotelID)
}

// GetHotelWithPhoto resolves hotel via mapping first, then name fallback, with layered caching.
func (s *HotelService) GetHotelWithPhoto(
	hotelID int,
	hotelName string,
	operator string,
	countryID int,
	mealPlan string,
) (*HotelWithPhoto, bool, error) {
	cacheKey := requestHotelCacheKey(operator, hotelID)
	if cached, ok := s.requestCache.Load(cacheKey); ok {
		result := cached.(cachedHotelResult)
		return result.hotel, result.fromMapping, nil
	}

	hotel, fromMapping, err := s.resolveHotel(hotelID, hotelName, operator, countryID, mealPlan)
	if err != nil || hotel == nil {
		return nil, fromMapping, err
	}

	withPhoto, err := s.buildHotelWithPhoto(hotel, fromMapping)
	if err != nil {
		return nil, fromMapping, err
	}

	s.requestCache.Store(cacheKey, cachedHotelResult{
		hotel:       withPhoto,
		fromMapping: fromMapping,
	})

	return withPhoto, fromMapping, nil
}

func (s *HotelService) buildHotelWithPhoto(hotel *cache.Hotel, fromMapping bool) (*HotelWithPhoto, error) {
	photos, err := getCachedHotelPhotos(s.DB, hotel.ID)
	if err != nil {
		return &HotelWithPhoto{
			ID:          hotel.ID,
			Name:        hotel.Name,
			FromMapping: fromMapping,
		}, nil
	}

	firstPhoto := ""
	if len(photos) > 0 {
		firstPhoto = photos[0].Image
	}

	return &HotelWithPhoto{
		ID:          hotel.ID,
		Name:        hotel.Name,
		Photo:       firstPhoto,
		Count:       len(photos),
		HotelPhotos: photos,
		FromMapping: fromMapping,
	}, nil
}

func (s *HotelService) resolveHotel(
	operatorHotelID int,
	hotelName string,
	operator string,
	countryID int,
	_ string,
) (*cache.Hotel, bool, error) {
	if err := PreloadHotelsCache(s.DB); err != nil {
		return nil, false, err
	}
	if err := PreloadHotelMappings(s.DB); err != nil {
		return nil, false, err
	}

	if hotel, err := FindHotelByMapping(operator, operatorHotelID); err == nil {
		return hotel, true, nil
	}

	hotel, score, err := FindHotelByNameWithScore(countryID, hotelName)
	if err != nil {
		return nil, false, err
	}

	if score < hotelMatchScoreThreshold {
		return nil, false, fmt.Errorf("hotel match score below threshold: %.2f", score)
	}

	EnqueueHotelMapping(HotelMappingJob{
		Operator:         operator,
		OperatorHotelID:  operatorHotelID,
		HotelID:          hotel.ID,
		HotelName:        hotelName,
		MatchedHotelName: hotel.Name,
		SimilarityScore:  score,
	})

	return hotel, false, nil
}
