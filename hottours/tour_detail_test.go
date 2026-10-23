package hottours

import "testing"

func TestDecodeTourDetailReadsPackagePrice(t *testing.T) {
	detail, err := decodeTourDetail([]byte(`{
		"id": "9928",
		"adults": 2,
		"childs": 0,
		"price": 1200,
		"date": "2026-11-03",
		"nights": 7,
		"roomType": "Standard",
		"hotel": {"id": 5, "name": "RED", "category": 4}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	ticket := mapTourDetail(detail, 12000)
	if ticket.PassengerCount != 2 || ticket.MinPerson != 2 || ticket.MaxPerson != 2 {
		t.Fatalf("passengers = %d min %d max %d", ticket.PassengerCount, ticket.MinPerson, ticket.MaxPerson)
	}
	if ticket.PriceFull <= 0 || ticket.OperatorPrice != "1200" {
		t.Fatalf("price_full=%d operator=%s", ticket.PriceFull, ticket.OperatorPrice)
	}
	if ticket.RoomType != "Standard" || ticket.DepartureDate != "20261103" {
		t.Fatalf("room=%s date=%s", ticket.RoomType, ticket.DepartureDate)
	}
}

func TestDecodeTourDetailUnwrapsData(t *testing.T) {
	detail, err := decodeTourDetail([]byte(`{"data":{"id":"77","adults":2,"price":10}}`))
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != "77" || detail.Adults != 2 {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestMapTourDetailDefaultsToTwoPassengers(t *testing.T) {
	ticket := mapTourDetail(tourDetail{ID: "1", Price: 100, Adults: 0}, 12000)
	if ticket.PassengerCount != 2 || ticket.MinPerson != 2 || ticket.MaxPerson != 2 {
		t.Fatalf("passengers = %d", ticket.PassengerCount)
	}
}
