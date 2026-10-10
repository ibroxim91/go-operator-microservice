package hottours

import (
	"testing"

	"go-operator-service/models"
)

func hotTicket(tourID string, hotelID, countryID, regionID, price int) *models.Ticket {
	return &models.Ticket{
		TourOperatorID: tourID,
		CountryID:      countryID,
		PriceFull:      price,
		Destination:    models.DestinationInfo{ID: regionID},
		TicketHotel:    []models.TicketHotel{{ID: hotelID}},
	}
}

func TestOrderHotToursKeepsCheapestHotelAndLeadsWithCountries(t *testing.T) {
	ordered := orderHotTours([]*models.Ticket{
		hotTicket("a", 1, 10, 100, 500),
		hotTicket("b", 1, 10, 100, 300),
		hotTicket("c", 2, 10, 200, 200),
		hotTicket("d", 3, 20, 300, 400),
		hotTicket("e", 4, 30, 400, 100),
		hotTicket("f", 5, 40, 500, 250),
		hotTicket("g", 6, 10, 600, 150),
	})
	if len(ordered) != 6 {
		t.Fatalf("deduped length = %d, want 6", len(ordered))
	}
	if ordered[0].TourOperatorID != "e" || ordered[1].TourOperatorID != "g" || ordered[2].TourOperatorID != "f" || ordered[3].TourOperatorID != "d" {
		t.Fatalf("featured = %s %s %s %s", ordered[0].TourOperatorID, ordered[1].TourOperatorID, ordered[2].TourOperatorID, ordered[3].TourOperatorID)
	}
	if ordered[4].TourOperatorID != "c" || ordered[5].TourOperatorID != "b" {
		t.Fatalf("rest = %s %s", ordered[4].TourOperatorID, ordered[5].TourOperatorID)
	}
}

func TestOrderHotToursFallsBackToRegions(t *testing.T) {
	ordered := orderHotTours([]*models.Ticket{
		hotTicket("a", 1, 10, 100, 100),
		hotTicket("b", 2, 10, 100, 200),
		hotTicket("c", 3, 10, 200, 300),
		hotTicket("d", 4, 10, 300, 400),
	})
	got := []string{ordered[0].TourOperatorID, ordered[1].TourOperatorID, ordered[2].TourOperatorID, ordered[3].TourOperatorID}
	want := []string{"a", "c", "d", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("featured = %v, want %v", got, want)
		}
	}
}

func TestDecodeHotToursAcceptsTopLevelArray(t *testing.T) {
	items, err := decodeHotTours([]byte(`[{"tourId":"abc","hotel":{"id":7,"name":"RED"},"price":100}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TourID != "abc" || items[0].Hotel.ID != 7 {
		t.Fatalf("items = %+v", items)
	}
}
