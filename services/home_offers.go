package services

import (
	"sort"

	"go-operator-service/models"
)

const homeOffersMaxTownsPerRegion = 3

// SelectHomeOfferTickets keeps 1 cheapest ticket per town and at most
// homeOffersMaxTownsPerRegion towns per destination region.
func SelectHomeOfferTickets(tickets []*models.Ticket) []*models.Ticket {
	if len(tickets) == 0 {
		return nil
	}

	type townKey struct {
		regionID int
		townID   int
		townName string
	}

	cheapestByTown := make(map[townKey]*models.Ticket)
	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		key := townKey{
			regionID: ticket.DestinationID,
			townID:   ticket.Destination.ID,
			townName: ticket.Destination.Name,
		}
		existing, ok := cheapestByTown[key]
		if !ok || ticket.PriceFull < existing.PriceFull {
			cheapestByTown[key] = ticket
		}
	}

	byRegion := make(map[int][]*models.Ticket)
	for key, ticket := range cheapestByTown {
		regionID := key.regionID
		byRegion[regionID] = append(byRegion[regionID], ticket)
	}

	selected := make([]*models.Ticket, 0, len(cheapestByTown))
	for _, regionTickets := range byRegion {
		sort.Slice(regionTickets, func(i, j int) bool {
			return regionTickets[i].PriceFull < regionTickets[j].PriceFull
		})
		limit := homeOffersMaxTownsPerRegion
		if len(regionTickets) < limit {
			limit = len(regionTickets)
		}
		selected = append(selected, regionTickets[:limit]...)
	}

	sort.Slice(selected, func(i, j int) bool {
		return selected[i].PriceFull < selected[j].PriceFull
	})
	return selected
}

// ApplyTicketVisaFlags sets visa_required from the in-memory country map.
func ApplyTicketVisaFlags(tickets []*models.Ticket) {
	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		ticket.VisaRequired = IsCountryVisaRequired(ticket.CountryID)
	}
}

// FilterTicketsByVisaRequired keeps tickets matching visa_required flag.
func FilterTicketsByVisaRequired(tickets []*models.Ticket, visaRequired bool) []*models.Ticket {
	filtered := make([]*models.Ticket, 0, len(tickets))
	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		if ticket.VisaRequired == visaRequired {
			filtered = append(filtered, ticket)
		}
	}
	return filtered
}

// BuildHomeOffersAsyncResult builds AsyncSamoResult for home-offers cache.
func BuildHomeOffersAsyncResult(tickets []*models.Ticket, totalFound int) *models.AsyncSamoResult {
	ApplyTicketVisaFlags(tickets)
	selected := SelectHomeOfferTickets(tickets)
	for _, ticket := range selected {
		ticket.FromCache = true
	}

	minPrice, maxPrice := ticketPriceRange(selected)
	pageSize := 100
	totalItems := totalFound
	if totalItems == 0 {
		totalItems = len(selected)
	}

	totalPages := 0
	if len(selected) > 0 {
		totalPages = len(selected) / pageSize
		if len(selected)%pageSize != 0 {
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
			Total:       len(selected),
			CurrentPage: 1,
			Results: models.AsyncSamoResultPayload{
				Tickets:             selected,
				MinPrice:            minPrice,
				MaxPrice:            maxPrice,
				Hotels:              BuildHotelSummaries(selected),
				HotelAmenities:      []string{},
				HotelFeaturesByType: []string{},
				HotelTypes:          []string{},
				TopDestinations:     []string{},
				TopDuration:         []string{},
			},
		},
	}
}

// CloneHomeOffersResult shallow-clones result with optional visa filter.
func CloneHomeOffersResult(cached *models.AsyncSamoResult, visaRequired *bool) *models.AsyncSamoResult {
	if cached == nil {
		return nil
	}

	tickets := cached.Data.Results.Tickets
	if visaRequired != nil {
		tickets = FilterTicketsByVisaRequired(tickets, *visaRequired)
	}

	marked := make([]*models.Ticket, len(tickets))
	copy(marked, tickets)
	for _, ticket := range marked {
		if ticket != nil {
			ticket.FromCache = true
		}
	}

	minPrice, maxPrice := ticketPriceRange(marked)
	pageSize := cached.Data.PageSize
	if pageSize == 0 {
		pageSize = 100
	}

	totalPages := 0
	if len(marked) > 0 {
		totalPages = len(marked) / pageSize
		if len(marked)%pageSize != 0 {
			totalPages++
		}
	}

	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Links:       cached.Data.Links,
			TotalItems:  len(marked),
			TotalPages:  totalPages,
			PageSize:    pageSize,
			Total:       len(marked),
			CurrentPage: 1,
			Results: models.AsyncSamoResultPayload{
				Tickets:             marked,
				MinPrice:            minPrice,
				MaxPrice:            maxPrice,
				Hotels:              BuildHotelSummaries(marked),
				HotelAmenities:      cached.Data.Results.HotelAmenities,
				HotelFeaturesByType: cached.Data.Results.HotelFeaturesByType,
				HotelTypes:          cached.Data.Results.HotelTypes,
				TopDestinations:     cached.Data.Results.TopDestinations,
				TopDuration:         cached.Data.Results.TopDuration,
			},
		},
	}
}
