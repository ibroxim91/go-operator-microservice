package services

import (
	"database/sql"
	"go-operator-service/repository"

)

type HotelService struct {
    DB *sql.DB
}

type HotelWithPhoto struct {
    ID    int
    Name  string
    Photo string
    Note  string
	Count int
}

// NewHotelService constructor
func NewHotelService(db *sql.DB) *HotelService {
    return &HotelService{DB: db}
}

// GetHotelWithPhoto: hotelni cache orqali topadi va birinchi rasmni qo‘shadi
func (s *HotelService) GetHotelWithPhoto(hotelID int, hotelName, operator string, countryID int, mealPlan string) (*HotelWithPhoto, bool, error) {
    hotel, exists, err := repository.GetHotelData(s.DB, hotelID, hotelName, operator, countryID, mealPlan)

    if err != nil || hotel == nil {
        return nil, exists, err
    }

    photo, err := repository.GetHotelPhoto(s.DB, hotel.ID)
    if err != nil {
        // Agar rasm topilmasa, faqat hotel qaytadi
        return &HotelWithPhoto{
            ID:   hotel.ID,
            Name: hotel.Name,
        }, exists, nil
    }

    return &HotelWithPhoto{
        ID:    hotel.ID,
        Name:  hotel.Name,
        Photo: photo.URL,
        Note:  photo.Note,
        Count:  photo.Count,
    }, exists, nil
}
