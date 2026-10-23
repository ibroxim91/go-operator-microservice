package turvisorsearch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestShouldSearch(t *testing.T) {
	if !ShouldSearch("") || !ShouldSearch("turvisor") || !ShouldSearch(" Turvisor ") {
		t.Fatal("empty and turvisor operators should search")
	}
	if ShouldSearch("samo_tour") || ShouldSearch("easy_booking") {
		t.Fatal("named SAMO operators should not search")
	}
}

func TestResolveRegionIDs(t *testing.T) {
	ids, ok := resolveRegionIDs(false, nil)
	if !ok || ids != nil {
		t.Fatalf("whole country = %v %v", ids, ok)
	}
	ids, ok = resolveRegionIDs(true, nil)
	if ok || ids != nil {
		t.Fatalf("missing region mapping = %v %v", ids, ok)
	}
	ids, ok = resolveRegionIDs(true, []int{6, 8})
	if !ok || len(ids) != 2 || ids[0] != 6 {
		t.Fatalf("mapped regions = %v %v", ids, ok)
	}
}

func TestDestinationRegionIDsIgnoreTowns(t *testing.T) {
	ids, ok := resolveRegionIDs(false, nil)
	if !ok || ids != nil {
		t.Fatalf("no destination = %v %v", ids, ok)
	}
	ids, ok = resolveRegionIDs(true, []int{77})
	if !ok || len(ids) != 1 || ids[0] != 77 {
		t.Fatalf("destination mapping = %v %v", ids, ok)
	}
	ids, ok = resolveRegionIDs(true, nil)
	if ok {
		t.Fatal("town subregion ids must not become regionIds")
	}
}

func TestClampDatesAndNights(t *testing.T) {
	from := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 40)
	from, to = clampDates(from, to)
	if to.Sub(from) != 20*24*time.Hour {
		t.Fatalf("date span = %s", to.Sub(from))
	}
	if from.Hour() != 0 {
		t.Fatalf("date was not truncated: %s", from)
	}

	nightsFrom, nightsTo := nightsWindow("1,20")
	if nightsFrom != 1 || nightsTo != 11 {
		t.Fatalf("nights = %d..%d", nightsFrom, nightsTo)
	}
	nightsFrom, nightsTo = nightsWindow("7,8,9,10,11,12,13,14")
	if nightsFrom != 7 || nightsTo != 14 {
		t.Fatalf("default nights = %d..%d", nightsFrom, nightsTo)
	}
}

func TestMakeURLUsesCharterAndCUWithoutToken(t *testing.T) {
	query := Query{
		DepartureID:   106,
		CountryID:     1,
		DateFrom:      time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		DateTo:        time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		NightsFrom:    7,
		NightsTo:      10,
		Adults:        2,
		Meal:          3,
		HotelCategory: 4,
		RegionIDs:     []int{6, 8},
		PriceFrom:     500,
		PriceTo:       2000,
	}
	raw := MakeURL(query)
	if strings.Contains(raw, "eyJ") || strings.Contains(strings.ToLower(raw), "bearer") {
		t.Fatalf("token leaked into url: %s", raw)
	}
	for _, part := range []string{
		"departureId=106",
		"countryId=1",
		"dateFrom=2026-10-12",
		"nightsFrom=7",
		"nightsTo=10",
		"currency=CU",
		"onlyCharter=true",
		"meal=3",
		"hotelCategory=4",
		"regionIds=6%2C8",
		"priceFrom=500",
		"priceTo=2000",
	} {
		if !strings.Contains(raw, part) {
			t.Fatalf("url %s missing %s", raw, part)
		}
	}
}

func TestValuesRepeatsChildrenAges(t *testing.T) {
	query := Query{
		DepartureID:  106,
		CountryID:    1,
		DateFrom:     time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		DateTo:       time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		NightsFrom:   7,
		NightsTo:     10,
		Adults:       2,
		Childs:       2,
		ChildrenAges: []int{5, 11},
	}
	values := query.Values()
	if values.Get("childs") != "2" {
		t.Fatalf("childs = %s", values.Get("childs"))
	}
	ages := values["childrenAges"]
	if len(ages) != 2 || ages[0] != "5" || ages[1] != "11" {
		t.Fatalf("childrenAges = %v", ages)
	}
}

func TestTourvisorChildrenKeepsFirstThree(t *testing.T) {
	childs, ages := tourvisorChildren(map[string]string{"CHILD": "4", "AGES": "5,11,4,8"})
	if childs != 3 || len(ages) != 3 || ages[0] != 5 || ages[1] != 11 || ages[2] != 4 {
		t.Fatalf("childs=%d ages=%v", childs, ages)
	}
}

func TestHotelFilterOnlyWithOperator(t *testing.T) {
	if includeHotelFilter("") {
		t.Fatal("empty operator must not pass hotel ids")
	}
	if !includeHotelFilter("turvisor") {
		t.Fatal("turvisor operator should pass hotel ids")
	}
	ids := hotelIDs("0,516,abc,77")
	if len(ids) != 2 || ids[0] != 516 || ids[1] != 77 {
		t.Fatalf("hotel ids = %v", ids)
	}
}

func TestDecodeSearchIDAndResults(t *testing.T) {
	id, err := decodeSearchID([]byte(`{"searchId":991}`))
	if err != nil || id != "991" {
		t.Fatalf("search id = %s %v", id, err)
	}
	id, err = decodeSearchID([]byte(`{"data":{"searchId":"42"}}`))
	if err != nil || id != "42" {
		t.Fatalf("wrapped search id = %s %v", id, err)
	}

	array, err := decodeResults([]byte(`[{"id":5,"name":"Hotel","tours":[{"id":"99","price":10}]}]`))
	if err != nil || len(array) != 1 || array[0].Tours[0].tourID() != "99" {
		t.Fatalf("array results = %#v %v", array, err)
	}
	wrapped, err := decodeResults([]byte(`{"data":[{"id":5,"name":"Hotel","tours":[{"tourId":77,"price":10}]}]}`))
	if err != nil || len(wrapped) != 1 || wrapped[0].Tours[0].tourID() != "77" {
		t.Fatalf("wrapped results = %#v %v", wrapped, err)
	}

	if !statusDone(searchStatus{Progress: 100}) {
		t.Fatal("progress 100 should finish")
	}
	if statusDone(decodeStatus([]byte(`{"data":{"progress":40,"status":"searching"}}`))) {
		t.Fatal("searching should continue")
	}
}

func TestMapSearchTourMatchesBookingShape(t *testing.T) {
	hotel := searchHotel{ID: 516, Name: "UNI SHARM", Category: 3, Picturelink: "//static.tourvisor.ru/hotel_pics/a.jpg"}
	hotel.Region.ID = 6
	hotel.Region.Name = "Шарм-эль-Шейх"
	hotel.Country.ID = 1
	hotel.Country.Name = "Египет"
	tour := searchTour{ID: "99283456631636", Date: "15.10.2026", Nights: 7, Price: 500, Adults: 2, RoomType: "Standard"}
	tour.Meal.Name = "AI"
	ticket := mapSearchTour(hotel, tour, Query{DepartureID: 106, OurCountryID: 4, Adults: 2}, 12000)
	if ticket == nil {
		t.Fatal("ticket is nil")
	}
	if ticket.Operator != "turvisor" || ticket.TourOperatorID != "99283456631636" {
		t.Fatalf("identity = %s %s", ticket.Operator, ticket.TourOperatorID)
	}
	if ticket.TicketHotel[0].ID != 516 || ticket.TicketHotel[0].MealPlan != "AI" {
		t.Fatalf("hotel = %#v", ticket.TicketHotel)
	}
	if ticket.DepartureTime != "20261015" || ticket.TravelTime != "20261022" || ticket.DepartureDate != "20261015" {
		t.Fatalf("dates = %s %s %s", ticket.DepartureTime, ticket.TravelTime, ticket.DepartureDate)
	}
	if ticket.Departure.Country != "Uzbekistan" || ticket.Departure.ID != 106 {
		t.Fatalf("departure = %#v", ticket.Departure)
	}
	if ticket.PriceFull <= 0 || ticket.Currency != "UZS" {
		t.Fatalf("price = %d %s", ticket.PriceFull, ticket.Currency)
	}
	if !strings.HasPrefix(ticket.HotelPhoto, "https://") {
		t.Fatalf("photo = %s", ticket.HotelPhoto)
	}
	encoded, err := json.Marshal(ticket)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(encoded)
	for _, part := range []string{`"extra_service":[]`, `"paid_extra_service":[]`, `"transports":[]`, `"tariff":[]`} {
		if !strings.Contains(raw, part) {
			t.Fatalf("missing %s in %s", part, raw)
		}
	}
	if strings.Contains(raw, "operator_link") || strings.Contains(raw, "operatorLink") {
		t.Fatalf("operator link leaked into ticket json: %s", raw)
	}
}
