package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-operator-service/cache"
	"go-operator-service/models"
)

const maxRecommendedTickets = 10
const normalPerRecommended = 3
const recommendedPriceBuckets = 3

// TicketSortMode controls in-memory ordering after search results are collected.
type TicketSortMode struct {
	Cheapest                 bool
	MostExpensive            bool
	RecommendedOnly          bool
	SkipRecommendedInterleave bool // home-offers: keep price order, no R/N/N/N boost
}

func ParseTicketSortMode(params map[string]string) TicketSortMode {
	if params == nil {
		return TicketSortMode{}
	}
	recommendedOnly := strings.EqualFold(strings.TrimSpace(params["recommended"]), "true")
	if recommendedOnly {
		return TicketSortMode{RecommendedOnly: true}
	}
	return TicketSortMode{
		Cheapest:      strings.EqualFold(strings.TrimSpace(params["cheapest"]), "true"),
		MostExpensive: strings.EqualFold(strings.TrimSpace(params["most_expensive"]), "true"),
	}
}

func MarkRecommendedFlags(tickets []*models.Ticket) {
	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		ticket.IsRecommended = cache.IsRecommendedHotel(ticket.HotelDBID)
	}
}

func sortTicketsByPrice(tickets []*models.Ticket, descending bool) {
	sort.SliceStable(tickets, func(i, j int) bool {
		a, b := tickets[i], tickets[j]
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		if descending {
			return a.PriceFull > b.PriceFull
		}
		return a.PriceFull < b.PriceFull
	})
}

// pickCheapestPerRecommendedHotel keeps one cheapest ticket per recommended hotel_db_id.
func pickCheapestPerRecommendedHotel(tickets []*models.Ticket) []*models.Ticket {
	bestByHotel := make(map[int]*models.Ticket)
	for _, ticket := range tickets {
		if ticket == nil || !ticket.IsRecommended || ticket.HotelDBID <= 0 {
			continue
		}
		existing, ok := bestByHotel[ticket.HotelDBID]
		if !ok || ticket.PriceFull < existing.PriceFull {
			bestByHotel[ticket.HotelDBID] = ticket
		}
	}
	out := make([]*models.Ticket, 0, len(bestByHotel))
	for _, ticket := range bestByHotel {
		out = append(out, ticket)
	}
	return out
}

// diversifyByPriceBuckets reorders tickets so cheap/mid/expensive mix across the list
// (round-robin across price thirds). Avoids pushing expensive recommendations to last pages.
func diversifyByPriceBuckets(tickets []*models.Ticket) []*models.Ticket {
	if len(tickets) <= 1 {
		return tickets
	}

	sorted := make([]*models.Ticket, len(tickets))
	copy(sorted, tickets)
	sortTicketsByPrice(sorted, false)

	n := len(sorted)
	bucketCount := recommendedPriceBuckets
	if n < bucketCount {
		bucketCount = n
	}

	buckets := make([][]*models.Ticket, bucketCount)
	for i, ticket := range sorted {
		bucketIdx := i * bucketCount / n
		if bucketIdx >= bucketCount {
			bucketIdx = bucketCount - 1
		}
		buckets[bucketIdx] = append(buckets[bucketIdx], ticket)
	}

	out := make([]*models.Ticket, 0, n)
	indexes := make([]int, bucketCount)
	for len(out) < n {
		progress := false
		for b := 0; b < bucketCount; b++ {
			if indexes[b] < len(buckets[b]) {
				out = append(out, buckets[b][indexes[b]])
				indexes[b]++
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	return out
}

// interleaveRecommended builds strict R,N,N,N,R,N,N,N... pattern.
// An R is only placed when at least normalPerRecommended normals remain after it.
// Leftover recommended tickets are appended only after all normals (end of list).
func interleaveRecommended(recommended, normal []*models.Ticket) []*models.Ticket {
	if len(recommended) == 0 {
		return normal
	}
	if len(normal) == 0 {
		return recommended
	}

	out := make([]*models.Ticket, 0, len(recommended)+len(normal))
	ri, ni := 0, 0

	for ri < len(recommended) {
		if len(normal)-ni < normalPerRecommended {
			break
		}
		out = append(out, recommended[ri])
		ri++
		for k := 0; k < normalPerRecommended; k++ {
			out = append(out, normal[ni])
			ni++
		}
	}

	if ni < len(normal) {
		out = append(out, normal[ni:]...)
	}
	if ri < len(recommended) {
		out = append(out, recommended[ri:]...)
	}
	return out
}

// lockRecommendedBadges ensures only tickets used as R slots keep is_recommended=true.
// Prevents alternate offers of recommended hotels (or rematching) from showing adjacent R badges.
func lockRecommendedBadges(ordered, recommendedSlots []*models.Ticket) {
	slotSet := make(map[*models.Ticket]struct{}, len(recommendedSlots))
	for _, ticket := range recommendedSlots {
		if ticket != nil {
			slotSet[ticket] = struct{}{}
		}
	}
	for _, ticket := range ordered {
		if ticket == nil {
			continue
		}
		_, ok := slotSet[ticket]
		ticket.IsRecommended = ok
	}
}

// ApplyTicketSortMode flags recommended hotels and builds the display order without extra operator requests.
// Default / price sorts: diversify recommended by price, then strict R + 3N interleave.
// Recommended-only: diversified recommended list.
// SkipRecommendedInterleave: price sort only (used by home-offers / hot tours).
func ApplyTicketSortMode(tickets []*models.Ticket, mode TicketSortMode) []*models.Ticket {
	if mode.SkipRecommendedInterleave {
		sortTicketsByPrice(tickets, mode.MostExpensive)
		for _, ticket := range tickets {
			if ticket != nil {
				ticket.IsRecommended = false
			}
		}
		return tickets
	}

	MarkRecommendedFlags(tickets)

	if mode.RecommendedOnly {
		rec := pickCheapestPerRecommendedHotel(tickets)
		rec = diversifyByPriceBuckets(rec)
		lockRecommendedBadges(rec, rec)
		return rec
	}

	recommended := pickCheapestPerRecommendedHotel(tickets)

	// Exclude every offer of recommended hotels from the normal pool so badges
	// cannot appear on N slots (which looked like R,R / R,N,R in the UI).
	normal := make([]*models.Ticket, 0, len(tickets))
	for _, ticket := range tickets {
		if ticket == nil || ticket.IsRecommended {
			continue
		}
		normal = append(normal, ticket)
	}

	recommended = diversifyByPriceBuckets(recommended)
	sortTicketsByPrice(normal, mode.MostExpensive)

	ordered := interleaveRecommended(recommended, normal)
	lockRecommendedBadges(ordered, recommended)
	return ordered
}

// FilterRecommendedTickets returns recommended tickets for the side list.
// Prefers tickets already marked is_recommended (after ApplyTicketSortMode lock).
// Falls back to hotel_db_id lookup without mutating flags on the input slice.
func FilterRecommendedTickets(tickets []*models.Ticket) []*models.Ticket {
	out := make([]*models.Ticket, 0)
	for _, ticket := range tickets {
		if ticket != nil && ticket.IsRecommended {
			out = append(out, ticket)
		}
	}
	if len(out) == 0 {
		for _, ticket := range tickets {
			if ticket != nil && cache.IsRecommendedHotel(ticket.HotelDBID) {
				out = append(out, ticket)
			}
		}
	}
	if len(out) > maxRecommendedTickets {
		out = out[:maxRecommendedTickets]
	}
	return out
}

func BuildAsyncSamoResult(results models.ResultResponse) *models.AsyncSamoResult {
	tickets := make([]*models.Ticket, len(results.Prices))
	copy(tickets, results.Prices)
	MarkRecommendedFlags(tickets)
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
	recommended := FilterRecommendedTickets(tickets)

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
			Links:            models.Links{Previous: nil, Next: nil},
			TotalItems:       totalItems,
			TotalPages:       pages,
			PageSize:         pageSize,
			Total:            results.Total,
			CurrentPage:      page,
			CurrentUsdCourse: results.CurrentUsdCourse,
			Results: models.AsyncSamoResultPayload{
				Tickets:             tickets,
				RecommendedTickets:  recommended,
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

func TakeCheapestTickets(tickets []*models.Ticket, limit int) []*models.Ticket {
	if limit <= 0 || len(tickets) == 0 {
		return nil
	}

	sorted := make([]*models.Ticket, len(tickets))
	copy(sorted, tickets)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PriceFull < sorted[j].PriceFull
	})

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	return sorted
}

func FilterPopularDestCacheTickets(
	tickets []*models.Ticket,
	departure string,
	destination string,
	countryID string,
) []*models.Ticket {
	departureID, _ := strconv.Atoi(strings.TrimSpace(departure))
	destinationID, _ := strconv.Atoi(strings.TrimSpace(destination))
	countryIDInt, _ := strconv.Atoi(strings.TrimSpace(countryID))

	filtered := make([]*models.Ticket, 0, len(tickets))
	for _, ticket := range tickets {
		if departureID > 0 && ticket.DepartureID != departureID {
			continue
		}
		if destinationID > 0 && ticket.DestinationID != destinationID {
			continue
		}
		if countryIDInt > 0 && destinationID == 0 && ticket.CountryID != countryIDInt {
			continue
		}
		filtered = append(filtered, ticket)
	}

	return filtered
}

func SlicePopularDestCacheResult(
	cached *models.StreamCacheResult,
	departure string,
	destination string,
	countryID string,
) *models.StreamCacheResult {
	if cached == nil {
		return nil
	}

	tickets := FilterPopularDestCacheTickets(
		cached.Tickets,
		departure,
		destination,
		countryID,
	)

	return &models.StreamCacheResult{
		Tickets: tickets,
		Hotels:  BuildHotelSummaries(tickets),
		Total:   len(tickets),
	}
}

func parseTicketDate(value string) (time.Time, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(normalized) >= 8 {
		normalized = normalized[:8]
	}
	return time.ParseInLocation("20060102", normalized, time.Local)
}
