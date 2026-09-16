package services

import (
	"testing"

	"go-operator-service/models"
)

func TestDiversifyByPriceBucketsMixesLowMidHigh(t *testing.T) {
	tickets := []*models.Ticket{
		{ID: 1, PriceFull: 100},
		{ID: 2, PriceFull: 200},
		{ID: 3, PriceFull: 300},
		{ID: 4, PriceFull: 400},
		{ID: 5, PriceFull: 500},
		{ID: 6, PriceFull: 600},
	}

	out := diversifyByPriceBuckets(tickets)
	if len(out) != len(tickets) {
		t.Fatalf("len=%d want %d", len(out), len(tickets))
	}

	// Round-robin across thirds: low(100,200), mid(300,400), high(500,600)
	// => 100,300,500,200,400,600
	want := []int{100, 300, 500, 200, 400, 600}
	for i, price := range want {
		if out[i].PriceFull != price {
			t.Fatalf("idx %d: got %d want %d", i, out[i].PriceFull, price)
		}
	}
}

func TestInterleaveRecommendedPattern(t *testing.T) {
	rec := []*models.Ticket{
		{ID: 1, PriceFull: 100, IsRecommended: true},
		{ID: 2, PriceFull: 500, IsRecommended: true},
	}
	norm := []*models.Ticket{
		{ID: 10, PriceFull: 110},
		{ID: 11, PriceFull: 120},
		{ID: 12, PriceFull: 130},
		{ID: 13, PriceFull: 140},
		{ID: 14, PriceFull: 150},
		{ID: 15, PriceFull: 160},
	}

	out := interleaveRecommended(rec, norm)
	// R N N N R N N N  (strict: only place R when 3 N remain)
	wantIDs := []int{1, 10, 11, 12, 2, 13, 14, 15}
	if len(out) != len(wantIDs) {
		t.Fatalf("len=%d want %d ids=%v", len(out), len(wantIDs), ticketIDs(out))
	}
	for i, id := range wantIDs {
		if out[i].ID != id {
			t.Fatalf("idx %d: got id %d want %d", i, out[i].ID, id)
		}
	}
}

func TestInterleaveNeverAdjacentRInPattern(t *testing.T) {
	rec := []*models.Ticket{
		{ID: 1, IsRecommended: true},
		{ID: 2, IsRecommended: true},
		{ID: 3, IsRecommended: true},
	}
	norm := []*models.Ticket{
		{ID: 10}, {ID: 11}, {ID: 12}, {ID: 13}, {ID: 14},
	}

	out := interleaveRecommended(rec, norm)
	// R N N N, then only 2 N left → no more R in pattern; leftover N then leftover R
	// => 1,10,11,12, 13,14, 2,3
	wantIDs := []int{1, 10, 11, 12, 13, 14, 2, 3}
	if len(out) != len(wantIDs) {
		t.Fatalf("len=%d want %d ids=%v", len(out), len(wantIDs), ticketIDs(out))
	}
	for i, id := range wantIDs {
		if out[i].ID != id {
			t.Fatalf("idx %d: got id %d want %d", i, out[i].ID, id)
		}
	}

	// No adjacent R before the trailing leftover block (after last normal).
	lastNorm := -1
	for i, ticket := range out {
		if !ticket.IsRecommended {
			lastNorm = i
		}
	}
	for i := 0; i < lastNorm; i++ {
		if out[i].IsRecommended && out[i+1].IsRecommended {
			t.Fatalf("adjacent R at %d,%d before last normal", i, i+1)
		}
	}
}

func TestApplyTicketSortModeSkipRecommendedInterleave(t *testing.T) {
	tickets := []*models.Ticket{
		{ID: 1, PriceFull: 300, HotelDBID: 1, IsRecommended: true},
		{ID: 2, PriceFull: 100, HotelDBID: 2, IsRecommended: false},
		{ID: 3, PriceFull: 200, HotelDBID: 3, IsRecommended: true},
	}

	out := ApplyTicketSortMode(tickets, TicketSortMode{SkipRecommendedInterleave: true})
	wantIDs := []int{2, 3, 1}
	if len(out) != len(wantIDs) {
		t.Fatalf("len=%d want %d ids=%v", len(out), len(wantIDs), ticketIDs(out))
	}
	for i, id := range wantIDs {
		if out[i].ID != id {
			t.Fatalf("idx %d: got id %d want %d (price order broken)", i, out[i].ID, id)
		}
		if out[i].IsRecommended {
			t.Fatalf("idx %d: is_recommended should be false for home-offers", i)
		}
	}
}

func ticketIDs(tickets []*models.Ticket) []int {
	ids := make([]int, len(tickets))
	for i, ticket := range tickets {
		ids[i] = ticket.ID
	}
	return ids
}
