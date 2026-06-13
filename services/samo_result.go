package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"go-operator-service/models"
)

func BuildAsyncSamoResult(results models.ResultResponse) *models.AsyncSamoResult {
	tickets := make([]*models.Ticket, len(results.Prices))
	copy(tickets, results.Prices)
	sort.Slice(tickets, func(i, j int) bool {
		return tickets[i].PriceFull < tickets[j].PriceFull
	})

	minPrice := 0
	maxPrice := 0
	if len(tickets) > 0 {
		minPrice = tickets[0].PriceFull
		maxPrice = tickets[0].PriceFull
		for _, ticket := range tickets {
			if ticket.PriceFull < minPrice {
				minPrice = ticket.PriceFull
			}
			if ticket.PriceFull > maxPrice {
				maxPrice = ticket.PriceFull
			}
		}
	}

	hotels := BuildHotelSummaries(tickets)

	pageSize := 100
	totalItems := len(tickets)
	page := results.Page
	if page == 0 {
		page = 1
	}

	pages := 0
	if totalItems > 0 {
		pages = totalItems / pageSize
		if totalItems%pageSize != 0 {
			pages++
		}
	}

	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Links:       models.Links{Previous: nil, Next: nil},
			TotalItems:  totalItems,
			TotalPages:  pages,
			PageSize:    pageSize,
			Total:       results.Total,
			CurrentPage: page,
			Results: models.AsyncSamoResultPayload{
				Tickets:             tickets,
				MinPrice:            minPrice,
				MaxPrice:            maxPrice,
				Hotels:              hotels,
				HotelAmenities:      []string{},
				HotelFeaturesByType: []string{},
				HotelTypes:          []string{},
				TopDestinations:     []string{},
				TopDuration:         []string{},
			},
		},
	}
}

func BuildHotelSummaries(tickets []*models.Ticket) []models.HotelSummary {
	hotelMap := map[string]models.HotelSummary{}
	hotels := make([]models.HotelSummary, 0)

	for _, ticket := range tickets {
		for _, hotel := range ticket.TicketHotel {
			key := fmt.Sprintf("%d|%s", hotel.ID, hotel.Name)
			if _, ok := hotelMap[key]; ok {
				continue
			}
			hotelMap[key] = models.HotelSummary{
				ID:          hotel.ID,
				Name:        hotel.Name,
				MealPlan:    hotel.MealPlan,
				Rating:      hotel.Rating,
				Operator:    ticket.Operator,
				Destination: ticket.Destination.Name,
			}
			hotels = append(hotels, hotelMap[key])
		}
	}

	return hotels
}

func BuildStreamCacheResult(tickets []*models.Ticket) *models.StreamCacheResult {
	marked := make([]*models.Ticket, len(tickets))
	copy(marked, tickets)
	for _, ticket := range marked {
		ticket.FromCache = true
	}

	return &models.StreamCacheResult{
		Tickets: marked,
		Hotels:  BuildHotelSummaries(marked),
		Total:   len(marked),
	}
}

func FilterTicketsByDateRange(
	tickets []*models.Ticket,
	dateFrom string,
	dateTo string,
) []*models.Ticket {
	from, err := parseTicketDate(dateFrom)
	if err != nil {
		return tickets
	}
	to, err := parseTicketDate(dateTo)
	if err != nil {
		return tickets
	}

	filtered := make([]*models.Ticket, 0, len(tickets))
	for _, ticket := range tickets {
		departureDate, err := parseTicketDate(ticket.DepartureDate)
		if err != nil {
			continue
		}
		if !departureDate.Before(from) && !departureDate.After(to) {
			filtered = append(filtered, ticket)
		}
	}

	return filtered
}

func ApplyPopularDestCacheResult(
	cached *models.StreamCacheResult,
	userSpecifiedDate bool,
	dateFrom string,
	dateTo string,
) *models.StreamCacheResult {
	tickets := cached.Tickets
	if userSpecifiedDate {
		tickets = FilterTicketsByDateRange(tickets, dateFrom, dateTo)
	}

	marked := make([]*models.Ticket, len(tickets))
	copy(marked, tickets)
	for _, ticket := range marked {
		ticket.FromCache = true
	}

	return &models.StreamCacheResult{
		Tickets: marked,
		Hotels:  BuildHotelSummaries(marked),
		Total:   len(marked),
	}
}

func parseTicketDate(value string) (time.Time, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(normalized) >= 8 {
		normalized = normalized[:8]
	}
	return time.ParseInLocation("20060102", normalized, time.Local)
}
