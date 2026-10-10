package hottours

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-operator-service/models"
	"go-operator-service/utils"
)

const hotToursURL = "https://api.tourvisor.ru/search/api/v1/tours/hots?departureId=106&countryIds=1,2,8,9,13,16,36&currency=USD&onlyCharter=true&limit=50"

const hotToursFeaturedCount = 4

type hotTourHotel struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Category    int     `json:"category"`
	Rating      float64 `json:"rating"`
	Picturelink string  `json:"picturelink"`
	Region      struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"region"`
	Country struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"country"`
}

type hotTourItem struct {
	TourID    string  `json:"tourId"`
	Price     float64 `json:"price"`
	PriceOld  float64 `json:"priceOld"`
	Currency  string  `json:"currency"`
	Date      string  `json:"date"`
	Nights    int     `json:"nights"`
	Departure struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"departure"`
	Country struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"country"`
	Hotel hotTourHotel `json:"hotel"`
	Meal  struct {
		Name     string `json:"name"`
		FullName string `json:"fullName"`
	} `json:"meal"`
}

type hotToursResponse struct {
	Data []hotTourItem `json:"data"`
}

// FetchHotTours loads the fixed hot-tours list and stores it as the existing ticket shape.
func FetchHotTours(ctx context.Context, usdCourse float64) (*models.AsyncSamoResult, error) {
	token := strings.TrimSpace(os.Getenv("TOURVISOR_JWT"))
	if token == "" {
		return nil, fmt.Errorf("TOURVISOR_JWT is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hotToursURL, nil)
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
		return nil, fmt.Errorf("hot tours status %d", resp.StatusCode)
	}
	items, err := decodeHotTours(body)
	if err != nil {
		return nil, err
	}
	tickets := make([]*models.Ticket, 0, len(items))
	for _, item := range items {
		tickets = append(tickets, mapHotTour(item, usdCourse))
	}
	ordered := orderHotTours(tickets)
	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			TotalItems:       len(ordered),
			TotalPages:       1,
			CurrentPage:      1,
			CurrentUsdCourse: usdCourse,
			Results: models.AsyncSamoResultPayload{
				Tickets:             ordered,
				RecommendedTickets:  []*models.Ticket{},
				Hotels:              []models.HotelSummary{},
				HotelAmenities:      []string{},
				HotelFeaturesByType: []string{},
				HotelTypes:          []string{},
				TopDestinations:     []string{},
				TopDuration:         []string{},
			},
		},
	}, nil
}

func decodeHotTours(body []byte) ([]hotTourItem, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []hotTourItem
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		return items, nil
	}
	var payload hotToursResponse
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

func mapHotTour(item hotTourItem, usdCourse float64) *models.Ticket {
	priceUZS := 0
	if usdCourse > 0 {
		priceUZS = utils.ConvertOperatorPriceToUzs(item.Price, usdCourse)
	}
	hotelID := item.Hotel.ID
	regionName := strings.TrimSpace(item.Hotel.Region.Name)
	countryName := strings.TrimSpace(item.Country.Name)
	if countryName == "" {
		countryName = strings.TrimSpace(item.Hotel.Country.Name)
	}
	photo := hotelPhotoURL(item.Hotel.Picturelink)
	rating := item.Hotel.Category
	if rating == 0 && item.Hotel.Rating > 0 {
		rating = int(math.Round(item.Hotel.Rating))
	}
	meal := strings.TrimSpace(item.Meal.Name)
	if meal == "" {
		meal = strings.TrimSpace(item.Meal.FullName)
	}
	nights := item.Nights
	if nights < 0 {
		nights = 0
	}
	return &models.Ticket{
		Title:           item.Hotel.Name,
		Slug:            hotTourSlug(item.Hotel.Name, item.TourID),
		DepartureDate:   item.Date,
		Nights:          nights,
		DurationDays:    nights,
		PassengerCount:  2,
		Price:           formatHotPriceMillion(priceUZS),
		PriceFull:       priceUZS,
		OperatorPrice:   strconv.FormatFloat(item.Price, 'f', -1, 64),
		Currency:        "UZS",
		Operator:        "turvisor",
		TourOperatorID:  item.TourID,
		CountryID:       item.Country.ID,
		DestinationID:   item.Hotel.Region.ID,
		HotelPhoto:      photo,
		HotelPhotoCount: boolToCount(photo != ""),
		HotelPhotos:     hotelPhotos(photo),
		TicketImages:    photo,
		Rating:          item.Hotel.Rating,
		Departure: models.DepartureInfo{
			ID:   item.Departure.ID,
			Name: item.Departure.Name,
		},
		Destination: models.DestinationInfo{
			ID:   item.Hotel.Region.ID,
			Name: regionName,
			Country: models.CountryInfo{
				ID:   item.Country.ID,
				Name: countryName,
			},
		},
		TicketHotel: []models.TicketHotel{{
			ID:       hotelID,
			Name:     item.Hotel.Name,
			Rating:   rating,
			MealPlan: meal,
		}},
	}
}

func hotelPhotoURL(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "//") {
		return "https:" + link
	}
	return link
}

func hotelPhotos(photo string) []models.HotelPhotos {
	if photo == "" {
		return nil
	}
	return []models.HotelPhotos{{Image: photo}}
}

func boolToCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

func formatHotPriceMillion(priceValueUsz int) string {
	if priceValueUsz <= 0 {
		return "0"
	}
	mln := float64(priceValueUsz) / 1_000_000
	if math.Mod(mln, 1) == 0 {
		return fmt.Sprintf("%d", int(mln))
	}
	return fmt.Sprintf("%.1f", mln)
}

func hotTourSlug(name, tourID string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "'", "", "\"", "")
	base = replacer.Replace(base)
	if base == "" {
		base = "tour"
	}
	if tourID != "" {
		return base + "-" + tourID
	}
	return base
}

func hotelKey(ticket *models.Ticket) string {
	if ticket == nil || len(ticket.TicketHotel) == 0 || ticket.TicketHotel[0].ID <= 0 {
		if ticket != nil && ticket.TourOperatorID != "" {
			return "tour:" + ticket.TourOperatorID
		}
		return ""
	}
	return strconv.Itoa(ticket.TicketHotel[0].ID)
}

func orderHotTours(tickets []*models.Ticket) []*models.Ticket {
	byHotel := map[string]*models.Ticket{}
	var order []string
	for _, ticket := range tickets {
		key := hotelKey(ticket)
		if key == "" {
			continue
		}
		prev, ok := byHotel[key]
		if !ok {
			byHotel[key] = ticket
			order = append(order, key)
			continue
		}
		if ticket.PriceFull < prev.PriceFull {
			byHotel[key] = ticket
		}
	}
	unique := make([]*models.Ticket, 0, len(order))
	for _, key := range order {
		unique = append(unique, byHotel[key])
	}
	sort.SliceStable(unique, func(i, j int) bool {
		if unique[i].PriceFull == unique[j].PriceFull {
			return unique[i].TourOperatorID < unique[j].TourOperatorID
		}
		return unique[i].PriceFull < unique[j].PriceFull
	})

	head := make([]*models.Ticket, 0, hotToursFeaturedCount)
	seenHotel := map[string]bool{}
	seenCountry := map[int]bool{}
	seenRegion := map[int]bool{}
	take := func(ticket *models.Ticket) {
		head = append(head, ticket)
		seenHotel[hotelKey(ticket)] = true
		if ticket.CountryID > 0 {
			seenCountry[ticket.CountryID] = true
		}
		if ticket.Destination.ID > 0 {
			seenRegion[ticket.Destination.ID] = true
		}
	}
	for _, ticket := range unique {
		if len(head) >= hotToursFeaturedCount {
			break
		}
		if ticket.CountryID == 0 || seenCountry[ticket.CountryID] || seenHotel[hotelKey(ticket)] {
			continue
		}
		take(ticket)
	}
	for _, ticket := range unique {
		if len(head) >= hotToursFeaturedCount {
			break
		}
		if seenHotel[hotelKey(ticket)] {
			continue
		}
		if ticket.Destination.ID != 0 && seenRegion[ticket.Destination.ID] {
			continue
		}
		take(ticket)
	}
	for _, ticket := range unique {
		if len(head) >= hotToursFeaturedCount {
			break
		}
		if seenHotel[hotelKey(ticket)] {
			continue
		}
		take(ticket)
	}

	inHead := map[string]bool{}
	for _, ticket := range head {
		inHead[hotelKey(ticket)] = true
	}
	rest := make([]*models.Ticket, 0, len(unique))
	for _, ticket := range unique {
		if inHead[hotelKey(ticket)] {
			continue
		}
		rest = append(rest, ticket)
	}
	return append(head, rest...)
}
