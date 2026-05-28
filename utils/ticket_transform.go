package utils

import (
	"fmt"
	"go-operator-service/models"
	"go-operator-service/services"
	"math"
	"os"
	"strconv"
	"strings"
)

func TransformSamoPriceToTicket(price models.Price, departure, operator, country, countrImageUrl string,
	currentUsdCourse float64, destinationID, departureID, countryID int, hotelService *services.HotelService, fromCache bool) *models.Ticket {

	priceValue := 0.0
	if val, err := strconv.ParseFloat(price.Price, 64); err == nil {
		priceValue = val
	}

	// kursga doimiy 200 qo‘shib hisoblash
	adjustedCourse := currentUsdCourse + 200
	priceValueUsz := int(priceValue * adjustedCourse)

	mln := float64(priceValueUsz) / 1_000_000

	var priceStr string
	if math.Mod(mln, 1) == 0 {
		priceStr = fmt.Sprintf("%d", int(mln))
	} else {
		priceStr = fmt.Sprintf("%.1f", mln)
	}


	hotelName := price.Hotel
	if strings.Contains(hotelName, "(") && strings.Contains(hotelName, ")") {
		start := strings.Index(hotelName, "(") + 1
		end := strings.Index(hotelName, ")")
		hotelName = hotelName[start:end]
	}
	hotel := GetHotel(&price, hotelName)

	slug := strings.ToLower(strings.ReplaceAll(price.Tour, " ", "-"))
	slug = strings.ReplaceAll(slug, "/", "-")
	slug += fmt.Sprintf("-%d", price.StateKey)
	if os.Getenv("TEST") == "true" {
		countrImageUrl = fmt.Sprintf("%s%s", os.Getenv("MEDIA_URL"), countrImageUrl)
	}
	ticket := &models.Ticket{
		TourOperatorID:    price.ID,
		ID:                price.TourKey,
		Title:             price.Tour,
		Currency:          price.Currency,
		HotelAvailability: price.HotelAvailability,
		Slug:              slug,
		CountryID:         countryID,
		Bron:              price.Bron,
		Nights:            price.Nights,
		Price:             priceStr,
		PriceFull:         priceValueUsz,
		RoomType:          price.Room,
		Place:             price.Place,
		FreightExternal:   price.FreightExternal,
		Operator:          operator,
		DepartureID:       departureID,
		DestinationID:     destinationID,
		DepartureTime:     price.CheckIn,
		Departure: models.DepartureInfo{
			ID:      price.TownFromKey,
			Name:    departure,
			Country: "Uzbekistan",
		},
		PassengerCount: price.Adult + price.Child,
		Rating:         4.5,
		DurationDays:   price.Nights,
		Destination: models.DestinationInfo{
			ID:   price.TownKey,
			Name: price.Town,
			Country: models.CountryInfo{
				ID:   price.StateKey,
				Name: country,
			},
		},
		TicketImages:    countrImageUrl,
		TicketAmenities: []string{},
		Badge:           []string{},
		VisaRequired:    false,
		FromCache:       fromCache,
		IsLiked:         false,
		TicketHotel:     hotel,

		// Qo‘shimcha fieldlar
		DepartureDate: price.CheckIn,
		TravelTime:    price.CheckOut,
		Languages:     "Русский, Английский",
		MinPerson:     1,
		MaxPerson:     price.Adult + price.Child,
		ImageBanner:   "",
		HotelInfo:     hotelName,
		HotelMeals:    price.Meal,
		AllowComment:  true,
		TicketIncludedServices: []models.IncludedService{
			{Image: "", Title: "Трансфер", Desc: "Трансфер из аэропорта в отель и обратно"},
		},
		TicketItinerary: []string{},
		TicketHotelMeals: []models.HotelMeal{
			{Image: "", Name: strings.ToUpper(price.Meal), Desc: "Тип питания"},
		},
		TravelAgencyID:   "samo_api",
		TicketComments:   []string{},
		Tariff:           []models.Tariff{{Name: "Стандарт"}},
		Transports:       []models.Transport{{ID: 0, Name: "Регулярный"}, {ID: 1, Name: "Чартер"}}, // list bo‘lib qoladi
		ExtraService:     []string{},
		PaidExtraService: []string{},
	}

	hotelWithPhoto, _, err := hotelService.GetHotelWithPhoto(
		price.HotelKey,
		hotelName,
		operator,
		countryID,
		strings.ToUpper(price.Meal),
	)

	if err == nil && hotelWithPhoto != nil {
		ticket.HotelPhoto = hotelWithPhoto.Photo
		ticket.HotelPhotoCount = hotelWithPhoto.Count
	}

	return ticket
}

func GetHotel(price *models.Price, hotelName string) []models.TicketHotel {
	// Default qiymat
	var starRating interface{} = 3
	hotelStarsExist := false

	if strings.HasPrefix(price.Star, "**") {
		starCount := 0
		for _, ch := range price.Star {
			if ch != '*' {
				break
			}
			starCount++
		}
		if starCount == len(price.Star) {
			starRating = starCount
			hotelStarsExist = true
		}
	}

	starStr := strings.ReplaceAll(price.Star, "*", "")
	starStr = strings.ReplaceAll(starStr, "HV-", "")

	if !hotelStarsExist {
		if strings.HasPrefix(starStr, "Special") {
			starRating = "Special Class"
		} else {
			if val, err := strconv.Atoi(starStr); err == nil {
				starRating = val
			}
		}
	}

	ticketHotel := models.TicketHotel{
		ID:       price.HotelKey,
		Name:     hotelName,
		MealPlan: strings.ToUpper(price.Meal),
		Rating:   starRating,
	}
	return []models.TicketHotel{ticketHotel}
}
