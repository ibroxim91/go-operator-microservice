package services

import (
	"testing"

	"go-operator-service/models"
)

func offerTicket(id int, operator string, hotelDBID int, operatorHotelID int, date string, nights int, price int) *models.Ticket {
	return &models.Ticket{
		ID:             id,
		TourOperatorID: operator + "-" + date,
		Operator:       operator,
		HotelDBID:      hotelDBID,
		DepartureDate:  date,
		Nights:         nights,
		PriceFull:      price,
		TicketHotel:    []models.TicketHotel{{ID: operatorHotelID, Name: "Hotel"}},
	}
}

func TestGroupKeepsCheapestOperatorPerHotel(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "malva_tour", 55, 701, "2026-11-10", 7, 950),
		offerTicket(3, "flykhiva", 55, 410, "2026-11-10", 7, 1100),
	}

	out := GroupTicketsByHotel(tickets)

	if len(out) != 1 {
		t.Fatalf("bitta karta kutilgan, %d chiqdi", len(out))
	}
	if out[0].ID != 2 {
		t.Fatalf("eng arzon taklif karta bo'lishi kerak, id=%d", out[0].ID)
	}
	if out[0].OffersCount != 3 {
		t.Fatalf("offers_count=%d want 3", out[0].OffersCount)
	}
	if out[0].Offers[0].PriceFull != 950 || out[0].Offers[2].PriceFull != 1200 {
		t.Fatalf("takliflar narx bo'yicha saralanmagan: %+v", out[0].Offers)
	}
}

func TestGroupKeepsDifferentDatesApart(t *testing.T) {
	// Kelishuv: sana boshqa bo'lsa — boshqa tur.
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "malva_tour", 55, 701, "2026-11-17", 7, 950),
	}

	if out := GroupTicketsByHotel(tickets); len(out) != 2 {
		t.Fatalf("ikkita karta kutilgan, %d chiqdi", len(out))
	}
}

func TestGroupKeepsDifferentNightsApart(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "samo_tour", 55, 900, "2026-11-10", 10, 1400),
	}

	if out := GroupTicketsByHotel(tickets); len(out) != 2 {
		t.Fatalf("ikkita karta kutilgan, %d chiqdi", len(out))
	}
}

// Eng muhim test: mapping'siz mehmonxonalar bitta kartaga yopishmasligi kerak.
func TestGroupNeverMergesUnmappedHotels(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 0, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "samo_tour", 0, 901, "2026-11-10", 7, 1300),
		offerTicket(3, "malva_tour", 0, 700, "2026-11-10", 7, 1100),
	}

	out := GroupTicketsByHotel(tickets)
	if len(out) != 3 {
		t.Fatalf("uchta karta kutilgan, %d chiqdi — noma'lum mehmonxonalar birlashib ketdi", len(out))
	}
}

func TestGroupMergesRoomAndMealVariantsOfOneOperator(t *testing.T) {
	a := offerTicket(1, "samo_tour", 0, 900, "2026-11-10", 7, 1200)
	a.RoomType = "DBL"
	b := offerTicket(2, "samo_tour", 0, 900, "2026-11-10", 7, 1050)
	b.RoomType = "SGL"

	out := GroupTicketsByHotel([]*models.Ticket{a, b})
	if len(out) != 1 {
		t.Fatalf("bitta karta kutilgan, %d chiqdi", len(out))
	}
	if out[0].ID != 2 {
		t.Fatalf("arzonrog'i tanlanishi kerak edi, id=%d", out[0].ID)
	}
}

func TestGroupIgnoresZeroPriceWhenChoosingWinner(t *testing.T) {
	// Ba'zi operatorlar narxni bo'sh qaytaradi; u 0 bo'lib "eng arzon"
	// bo'lib qolmasligi kerak.
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 0),
		offerTicket(2, "malva_tour", 55, 701, "2026-11-10", 7, 980),
	}

	out := GroupTicketsByHotel(tickets)
	if len(out) != 1 || out[0].ID != 2 {
		t.Fatalf("narxi bor taklif tanlanishi kerak edi: %+v", out)
	}
}

func TestGroupIsIdempotent(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "malva_tour", 55, 701, "2026-11-10", 7, 950),
	}

	once := GroupTicketsByHotel(tickets)
	twice := GroupTicketsByHotel(once)

	if len(twice) != 1 {
		t.Fatalf("bitta karta kutilgan, %d chiqdi", len(twice))
	}
	if twice[0].OffersCount != 2 {
		t.Fatalf("takrorlangan guruhlash takliflarni buzdi: %d ta, %+v",
			twice[0].OffersCount, twice[0].Offers)
	}
}

func TestGroupPreservesInputOrder(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 77, 900, "2026-11-10", 7, 1500),
		offerTicket(2, "samo_tour", 55, 901, "2026-11-10", 7, 900),
	}

	out := GroupTicketsByHotel(tickets)
	if len(out) != 2 || out[0].HotelDBID != 77 || out[1].HotelDBID != 55 {
		t.Fatalf("kirish tartibi saqlanmadi: %+v", out)
	}
}

func TestGroupDropsRepeatedIdenticalOffers(t *testing.T) {
	// Operator aynan bir xil taklifni ikki marta qaytaradi — bron id'si
	// ham bir xil. Kartada u bir marta ko'rinishi kerak.
	a := offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200)
	a.TourOperatorID = "0xAAA"
	b := offerTicket(2, "samo_tour", 55, 900, "2026-11-10", 7, 1200)
	b.TourOperatorID = "0xAAA"

	out := GroupTicketsByHotel([]*models.Ticket{a, b})
	if len(out) != 1 {
		t.Fatalf("bitta karta kutilgan, %d chiqdi", len(out))
	}
	if out[0].OffersCount != 1 {
		t.Fatalf("takror taklif qolib ketdi: %+v", out[0].Offers)
	}
}

func TestGroupKeepsOffersWithDifferentBookingIds(t *testing.T) {
	// Narx, xona va ovqat bir xil bo'lsa ham, bron id'si boshqa bo'lsa bu
	// boshqa taklif — masalan boshqa reys. Uni yo'qotib bo'lmaydi.
	a := offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200)
	a.TourOperatorID = "0xAAA"
	b := offerTicket(2, "samo_tour", 55, 900, "2026-11-10", 7, 1200)
	b.TourOperatorID = "0xBBB"

	out := GroupTicketsByHotel([]*models.Ticket{a, b})
	if len(out) != 1 || out[0].OffersCount != 2 {
		t.Fatalf("ikkala taklif ham qolishi kerak edi: %+v", out[0].Offers)
	}
}

func TestGroupKeepsOffersWithoutBookingId(t *testing.T) {
	a := offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200)
	a.TourOperatorID = ""
	b := offerTicket(2, "samo_tour", 55, 900, "2026-11-10", 7, 1300)
	b.TourOperatorID = ""

	out := GroupTicketsByHotel([]*models.Ticket{a, b})
	if len(out) != 1 || out[0].OffersCount != 2 {
		t.Fatalf("id'siz takliflar qoldirilishi kerak: %+v", out[0].Offers)
	}
}
