package hottours

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go-operator-service/models"
	"go-operator-service/utils"
)

// Tour card is loaded in CU, the same currency the Tourvisor tour endpoint accepts.
const tourDetailURL = "https://api.tourvisor.ru/search/api/v1/tours/%s?currency=CU"

type tourDetail struct {
	ID        string  `json:"id"`
	Adults    int     `json:"adults"`
	Childs    int     `json:"childs"`
	Price     float64 `json:"price"`
	Date      string  `json:"date"`
	Nights    int     `json:"nights"`
	RoomType  string  `json:"roomType"`
	Picture   string  `json:"picture"`
	Placement string  `json:"placement"`
	Departure struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"departure"`
	Hotel struct {
		ID       int     `json:"id"`
		Name     string  `json:"name"`
		Category int     `json:"category"`
		Rating   float64 `json:"rating"`
		Region   struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"region"`
		Country struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"country"`
	} `json:"hotel"`
	Meal struct {
		Name     string `json:"name"`
		FullName string `json:"fullName"`
	} `json:"meal"`
}

// FetchTour loads one Tourvisor tour. Hot-list prices are per person; this
// payload is the package price for the adults on the tour.
func FetchTour(ctx context.Context, tourID string, usdCourse float64) (*models.Ticket, error) {
	tourID = strings.TrimSpace(tourID)
	if !validTourID(tourID) {
		return nil, fmt.Errorf("invalid tour id")
	}
	token := strings.TrimSpace(os.Getenv("TOURVISOR_JWT"))
	if token == "" {
		return nil, fmt.Errorf("TOURVISOR_JWT is empty")
	}
	endpoint := fmt.Sprintf(tourDetailURL, url.PathEscape(tourID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tour detail status %d", resp.StatusCode)
	}
	detail, err := decodeTourDetail(body)
	if err != nil {
		return nil, err
	}
	if detail.Price <= 0 {
		return nil, fmt.Errorf("tour detail has no price")
	}
	return mapTourDetail(detail, usdCourse), nil
}

func validTourID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func decodeTourDetail(body []byte) (tourDetail, error) {
	var direct tourDetail
	if err := json.Unmarshal(body, &direct); err != nil {
		return tourDetail{}, err
	}
	if strings.TrimSpace(direct.ID) != "" {
		return direct, nil
	}
	var wrapped struct {
		Data tourDetail `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return tourDetail{}, err
	}
	if strings.TrimSpace(wrapped.Data.ID) == "" {
		return tourDetail{}, fmt.Errorf("tour detail is empty")
	}
	return wrapped.Data, nil
}

func mapTourDetail(detail tourDetail, usdCourse float64) *models.Ticket {
	priceUZS := 0
	if usdCourse > 0 {
		priceUZS = utils.ConvertOperatorPriceToUzs(detail.Price, usdCourse)
	}
	passengers := detail.Adults + detail.Childs
	if passengers < 1 {
		passengers = 2
	}
	nights := detail.Nights
	if nights < 0 {
		nights = 0
	}
	checkIn := normalizeHotTourDate(detail.Date)
	checkOut := addHotTourNights(checkIn, nights)
	meal := strings.TrimSpace(detail.Meal.Name)
	if meal == "" {
		meal = strings.TrimSpace(detail.Meal.FullName)
	}
	if meal == "" {
		meal = "RO"
	}
	photo := hotelPhotoURL(detail.Picture)
	rating := detail.Hotel.Category
	name := strings.TrimSpace(detail.Hotel.Name)
	return &models.Ticket{
		Title:          name,
		Slug:           hotTourSlug(name, detail.ID),
		DepartureDate:  checkIn,
		DepartureTime:  checkIn,
		TravelTime:     checkOut,
		Nights:         nights,
		DurationDays:   nights,
		PassengerCount: passengers,
		Adults:         detail.Adults,
		Childs:         detail.Childs,
		MinPerson:      passengers,
		MaxPerson:      passengers,
		Price:          formatHotPriceMillion(priceUZS),
		PriceFull:      priceUZS,
		OperatorPrice:  strconv.FormatFloat(detail.Price, 'f', -1, 64),
		Currency:       "UZS",
		Operator:       "turvisor",
		TourOperatorID: detail.ID,
		HotelPhoto:     photo,
		RoomType:       strings.TrimSpace(detail.RoomType),
		Place:          strings.TrimSpace(detail.Placement),
		HotelMeals:     meal,
		Departure: models.DepartureInfo{
			ID:      detail.Departure.ID,
			Name:    detail.Departure.Name,
			Country: "Uzbekistan",
		},
		Destination: models.DestinationInfo{
			ID:   detail.Hotel.Region.ID,
			Name: strings.TrimSpace(detail.Hotel.Region.Name),
			Country: models.CountryInfo{
				ID:   detail.Hotel.Country.ID,
				Name: strings.TrimSpace(detail.Hotel.Country.Name),
			},
		},
		TicketHotel: []models.TicketHotel{{
			ID:       detail.Hotel.ID,
			Name:     name,
			Rating:   rating,
			MealPlan: meal,
		}},
	}
}
