package services

import (
	"math/rand"
	"sort"
	"time"

	"go-operator-service/models"
)

const popularDestMaxTicketsPerDestination = 10

// BuildPopularDestAsyncResult builds home-friendly ticket order and AsyncSamoData payload.
func BuildPopularDestAsyncResult(
	perDestinationTickets [][]*models.Ticket,
	totalFound int,
) *models.AsyncSamoResult {
	cheapestPerDest := make([]*models.Ticket, 0, len(perDestinationTickets))
	rest := make([]*models.Ticket, 0)

	for _, destTickets := range perDestinationTickets {
		sorted := TakeCheapestTickets(destTickets, popularDestMaxTicketsPerDestination)
		if len(sorted) == 0 {
			continue
		}
		cheapestPerDest = append(cheapestPerDest, sorted[0])
		if len(sorted) > 1 {
			rest = append(rest, sorted[1:]...)
		}
	}

	sort.Slice(cheapestPerDest, func(i, j int) bool {
		return cheapestPerDest[i].PriceFull < cheapestPerDest[j].PriceFull
	})

	if len(rest) > 1 {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		rng.Shuffle(len(rest), func(i, j int) {
			rest[i], rest[j] = rest[j], rest[i]
		})
	}

	finalTickets := make([]*models.Ticket, 0, len(cheapestPerDest)+len(rest))
	finalTickets = append(finalTickets, cheapestPerDest...)
	finalTickets = append(finalTickets, rest...)

	for _, ticket := range finalTickets {
		ticket.FromCache = true
	}

	minPrice, maxPrice := ticketPriceRange(finalTickets)
	pageSize := 100
	totalItems := totalFound
	if totalItems == 0 {
		totalItems = len(finalTickets)
	}

	totalPages := 0
	if len(finalTickets) > 0 {
		totalPages = len(finalTickets) / pageSize
		if len(finalTickets)%pageSize != 0 {
			totalPages++
		}
	}

	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Links:       models.Links{Previous: nil, Next: nil},
			TotalItems:  totalItems,
			TotalPages:  totalPages,
			PageSize:    pageSize,
			Total:       len(finalTickets),
			CurrentPage: 1,
			Results: models.AsyncSamoResultPayload{
				Tickets:             finalTickets,
				MinPrice:            minPrice,
				MaxPrice:            maxPrice,
				Hotels:              BuildHotelSummaries(finalTickets),
				HotelAmenities:      []string{},
				HotelFeaturesByType: []string{},
				HotelTypes:          []string{},
				TopDestinations:     []string{},
				TopDuration:         []string{},
			},
		},
	}
}

func FilterPopularDestAsyncResult(
	cached *models.AsyncSamoResult,
	departure string,
	destination string,
	countryID string,
	userSpecifiedDate bool,
	dateFrom string,
	dateTo string,
) *models.AsyncSamoResult {
	if cached == nil {
		return nil
	}

	tickets := FilterPopularDestCacheTickets(
		cached.Data.Results.Tickets,
		departure,
		destination,
		countryID,
	)
	if userSpecifiedDate {
		tickets = FilterTicketsByDateRange(tickets, dateFrom, dateTo)
	}

	for _, ticket := range tickets {
		ticket.FromCache = true
	}

	minPrice, maxPrice := ticketPriceRange(tickets)
	pageSize := cached.Data.PageSize
	if pageSize == 0 {
		pageSize = 100
	}

	totalPages := 0
	if len(tickets) > 0 {
		totalPages = len(tickets) / pageSize
		if len(tickets)%pageSize != 0 {
			totalPages++
		}
	}

	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Links:       cached.Data.Links,
			TotalItems:  cached.Data.TotalItems,
			TotalPages:  totalPages,
			PageSize:    pageSize,
			Total:       len(tickets),
			CurrentPage: cached.Data.CurrentPage,
			Results: models.AsyncSamoResultPayload{
				Tickets:             tickets,
				MinPrice:            minPrice,
				MaxPrice:            maxPrice,
				Hotels:              BuildHotelSummaries(tickets),
				HotelAmenities:      cached.Data.Results.HotelAmenities,
				HotelFeaturesByType: cached.Data.Results.HotelFeaturesByType,
				HotelTypes:          cached.Data.Results.HotelTypes,
				TopDestinations:     cached.Data.Results.TopDestinations,
				TopDuration:         cached.Data.Results.TopDuration,
			},
		},
	}
}

func ticketPriceRange(tickets []*models.Ticket) (int, int) {
	if len(tickets) == 0 {
		return 0, 0
	}

	minPrice := tickets[0].PriceFull
	maxPrice := tickets[0].PriceFull
	for _, ticket := range tickets {
		if ticket.PriceFull < minPrice {
			minPrice = ticket.PriceFull
		}
		if ticket.PriceFull > maxPrice {
			maxPrice = ticket.PriceFull
		}
	}
	return minPrice, maxPrice
}
