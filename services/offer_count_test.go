package services

import (
	"testing"

	"go-operator-service/models"
)

func TestCountOffersAddsGroupedAndSingleCards(t *testing.T) {
	tickets := []*models.Ticket{
		{OffersCount: 558}, // bitta mehmonxona, 558 taklif
		{},                 // guruhlanmagan karta — bitta taklif
		nil,
		{OffersCount: 3},
	}
	if got := CountOffers(tickets); got != 562 {
		t.Fatalf("CountOffers = %d, 562 kutilgan", got)
	}
}

func TestOperatorPagesToFetch(t *testing.T) {
	cases := map[int]int{1: 1, 9: 9, 37: 37, 60: 60, 500: 60}
	for total, want := range cases {
		if got := OperatorPagesToFetch(total); got != want {
			t.Errorf("OperatorPagesToFetch(%d) = %d, %d kutilgan", total, got, want)
		}
	}
	t.Setenv("OPERATOR_MAX_PAGES", "20")
	if got := OperatorPagesToFetch(37); got != 20 {
		t.Fatalf("OPERATOR_MAX_PAGES e'tiborga olinmadi: %d", got)
	}
}
