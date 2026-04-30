package utils

import (
	"fmt"
	"go-operator-service/models"
	"math"
	"strconv"
	"strings"
)

func TransformSamoPriceToTicket(price models.Price, departure, operator, country, countrImageUrl string, destinationID, departureID int, fromCache bool) *models.Ticket {
    // USD kursini olish (mock)
    currentUsdCourse := 12000 // misol uchun
    priceValue := 0.0
    if val, err := strconv.ParseFloat(price.Price, 64); err == nil {
        priceValue = val
    }
    priceValueUsz := int(priceValue * float64(currentUsdCourse+200))
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

    ticket := &models.Ticket{
        TourOperatorID: price.ID,
        ID:             price.TourKey, // yoki hash
        Title:          price.Tour,
        Slug:           slug,
        Nights:         price.Nights,
        Price:          priceStr,
        PriceFull:      priceValueUsz,
        Operator:       operator,
        DepartureID:    departureID,
        DestinationID:  destinationID,
        DepartureTime:  price.CheckIn,
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
                Name: country, // mapping qo‘shish mumkin
            },
        },
        TicketImages:   countrImageUrl,
        TicketAmenities: []string{},
        Badge:          []string{},
        VisaRequired:   false,
        FromCache:      fromCache,
        IsLiked:        false,
        TicketHotel: hotel,
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
