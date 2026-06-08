package services

import (
	"database/sql"
	"go-operator-service/models"
	"go-operator-service/repository"
	// "log"
)

type HotelService struct {
	DB *sql.DB
}

type HotelWithPhoto struct {
	ID          int
	Name        string
	Photo       string
	Count       int
	HotelPhotos []models.HotelPhotos
}

// NewHotelService constructor
func NewHotelService(db *sql.DB) *HotelService {
	return &HotelService{DB: db}
}

// GetHotelWithPhoto: hotelni cache orqali topadi va birinchi rasmni qo‘shadi
func (s *HotelService) GetHotelWithPhoto(hotelID int, hotelName, operator string, countryID int, mealPlan string) (*HotelWithPhoto, bool, error) {
	hotel, exists, err := GetHotelData(s.DB, hotelID, hotelName, operator, countryID, mealPlan)

	if err != nil || hotel == nil {
		return nil, exists, err
	}

	photos, err := repository.GetHotelPhotos(s.DB, hotel.ID)
	if err != nil {
		// Agar rasm topilmasa, faqat hotel qaytadi
		
		return &HotelWithPhoto{
			ID:   hotel.ID,
			Name: hotel.Name,
		}, exists, nil
	}
	fitsImage := ""
	if len(photos) > 0 {
		fitsImage = photos[0].Image

	}
	return &HotelWithPhoto{
		ID:          hotel.ID,
		Name:        hotel.Name,
		Photo:       fitsImage,
		Count:       len(photos),
		HotelPhotos: []models.HotelPhotos{},
	}, exists, nil
}
