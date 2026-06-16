package services

import (
	"database/sql"
	"sync"

	"go-operator-service/models"
	"go-operator-service/repository"
)

var (
	hotelPhotosCache   = map[int][]models.HotelPhotos{}
	hotelPhotosCacheMu sync.RWMutex
)

func getCachedHotelPhotos(db *sql.DB, hotelID int) ([]models.HotelPhotos, error) {
	hotelPhotosCacheMu.RLock()
	if photos, ok := hotelPhotosCache[hotelID]; ok {
		hotelPhotosCacheMu.RUnlock()
		return photos, nil
	}
	hotelPhotosCacheMu.RUnlock()

	photos, err := repository.GetHotelPhotos(db, hotelID)
	if err != nil {
		return nil, err
	}

	hotelPhotosCacheMu.Lock()
	hotelPhotosCache[hotelID] = photos
	hotelPhotosCacheMu.Unlock()

	return photos, nil
}
