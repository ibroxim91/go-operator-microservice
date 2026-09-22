package services

import (
	"testing"

	"go-operator-service/cache"
	"go-operator-service/models"
)

func TestSelectRecommendedHomeOfferTickets(t *testing.T) {
	cache.SetRecommendedHotelIDs(map[int]struct{}{
		10: {},
		20: {},
		30: {},
	})

	tickets := []*models.Ticket{
		{ID: 1, HotelDBID: 10, PriceFull: 5_000_000},
		{ID: 2, HotelDBID: 10, PriceFull: 4_000_000}, // cheapest for hotel 10
		{ID: 3, HotelDBID: 20, PriceFull: 6_000_000},
		{ID: 4, HotelDBID: 99, PriceFull: 1_000_000}, // not recommended
		{ID: 5, HotelDBID: 30, PriceFull: 3_500_000},
	}

	selected := SelectRecommendedHomeOfferTickets(tickets)
	if len(selected) != 3 {
		t.Fatalf("expected 3 recommended hotels, got %d", len(selected))
	}
	if selected[0].HotelDBID != 30 || selected[0].PriceFull != 3_500_000 {
		t.Fatalf("expected cheapest first hotel 30 @ 3500000, got hotel=%d price=%d", selected[0].HotelDBID, selected[0].PriceFull)
	}
	if selected[1].HotelDBID != 10 || selected[1].PriceFull != 4_000_000 {
		t.Fatalf("expected hotel 10 cheapest ticket, got hotel=%d price=%d", selected[1].HotelDBID, selected[1].PriceFull)
	}
	for _, ticket := range selected {
		if !ticket.IsRecommended {
			t.Fatalf("ticket %d should keep is_recommended=true", ticket.ID)
		}
	}
}

func TestSelectRecommendedHomeOfferTicketsLimit(t *testing.T) {
	ids := make(map[int]struct{}, homeOffersRecommendedMaxTickets+5)
	tickets := make([]*models.Ticket, 0, homeOffersRecommendedMaxTickets+5)
	for i := 1; i <= homeOffersRecommendedMaxTickets+5; i++ {
		ids[i] = struct{}{}
		tickets = append(tickets, &models.Ticket{
			ID:         i,
			HotelDBID:  i,
			PriceFull:  i * 1000,
		})
	}
	cache.SetRecommendedHotelIDs(ids)

	selected := SelectRecommendedHomeOfferTickets(tickets)
	if len(selected) != homeOffersRecommendedMaxTickets {
		t.Fatalf("expected max %d tickets, got %d", homeOffersRecommendedMaxTickets, len(selected))
	}
}
