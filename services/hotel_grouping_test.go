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

	out := GroupTicketsByHotel(tickets, false)

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

func TestGroupMergeDatesPutsSameHotelOnOneCard(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "turvisor", 0, 516, "20261026", 7, 15_700_000),
		offerTicket(2, "turvisor", 0, 516, "20261027", 7, 15_700_000),
	}

	out := GroupTicketsByHotelMergeDates(tickets)
	if len(out) != 1 {
		t.Fatalf("bitta karta kutilgan, %d chiqdi", len(out))
	}
	if out[0].OffersCount != 2 {
		t.Fatalf("ikkala sana offers ichida qolishi kerak: %+v", out[0].Offers)
	}
}

func TestGroupKeepsDifferentDatesApart(t *testing.T) {
	// Kelishuv: sana boshqa bo'lsa — boshqa tur.
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "malva_tour", 55, 701, "2026-11-17", 7, 950),
	}

	if out := GroupTicketsByHotel(tickets, false); len(out) != 2 {
		t.Fatalf("ikkita karta kutilgan, %d chiqdi", len(out))
	}
}

func TestGroupKeepsDifferentNightsApart(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "samo_tour", 55, 900, "2026-11-10", 10, 1400),
	}

	if out := GroupTicketsByHotel(tickets, false); len(out) != 2 {
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

	out := GroupTicketsByHotel(tickets, false)
	if len(out) != 3 {
		t.Fatalf("uchta karta kutilgan, %d chiqdi — noma'lum mehmonxonalar birlashib ketdi", len(out))
	}
}

func TestGroupMergesRoomAndMealVariantsOfOneOperator(t *testing.T) {
	a := offerTicket(1, "samo_tour", 0, 900, "2026-11-10", 7, 1200)
	a.RoomType = "DBL"
	b := offerTicket(2, "samo_tour", 0, 900, "2026-11-10", 7, 1050)
	b.RoomType = "SGL"

	out := GroupTicketsByHotel([]*models.Ticket{a, b}, false)
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

	out := GroupTicketsByHotel(tickets, false)
	if len(out) != 1 || out[0].ID != 2 {
		t.Fatalf("narxi bor taklif tanlanishi kerak edi: %+v", out)
	}
}

func TestGroupIsIdempotent(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200),
		offerTicket(2, "malva_tour", 55, 701, "2026-11-10", 7, 950),
	}

	once := GroupTicketsByHotel(tickets, false)
	twice := GroupTicketsByHotel(once, false)

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

	out := GroupTicketsByHotel(tickets, false)
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

	out := GroupTicketsByHotel([]*models.Ticket{a, b}, false)
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

	out := GroupTicketsByHotel([]*models.Ticket{a, b}, false)
	if len(out) != 1 || out[0].OffersCount != 2 {
		t.Fatalf("ikkala taklif ham qolishi kerak edi: %+v", out[0].Offers)
	}
}

func TestGroupKeepsOffersWithoutBookingId(t *testing.T) {
	a := offerTicket(1, "samo_tour", 55, 900, "2026-11-10", 7, 1200)
	a.TourOperatorID = ""
	b := offerTicket(2, "samo_tour", 55, 900, "2026-11-10", 7, 1300)
	b.TourOperatorID = ""

	out := GroupTicketsByHotel([]*models.Ticket{a, b}, false)
	if len(out) != 1 || out[0].OffersCount != 2 {
		t.Fatalf("id'siz takliflar qoldirilishi kerak: %+v", out[0].Offers)
	}
}

// Sana oralig'i bo'yicha qidiruvda bitta mehmonxona bitta karta bo'lishi
// kerak. Productionda o'lchandi: 85 ta kartaning hammasi atigi 10 ta
// mehmonxona edi, GalaTower Hotel 20 marta takrorlangan — har sana va
// har kecha soni alohida karta bo'lgani uchun.
func TestGroupAcrossDatesCollapsesOneHotelToOneCard(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "malva_tour", 77, 500, "20261111", 7, 1200),
		offerTicket(2, "malva_tour", 77, 500, "20261112", 9, 1000),
		offerTicket(3, "malva_tour", 77, 500, "20261114", 12, 1500),
		offerTicket(4, "malva_tour", 88, 501, "20261111", 7, 900),
	}

	out := GroupTicketsByHotel(tickets, true)

	if len(out) != 2 {
		t.Fatalf("ikkita mehmonxona ikkita karta bo'lishi kerak, chiqdi: %d", len(out))
	}
	var first *models.Ticket
	for _, ticket := range out {
		if ticket.HotelDBID == 77 {
			first = ticket
		}
	}
	if first == nil {
		t.Fatal("77-mehmonxona yo'qoldi")
	}
	if first.PriceFull != 1000 {
		t.Fatalf("eng arzon sana tanlanishi kerak edi, narx: %d", first.PriceFull)
	}
	if first.OffersCount != 3 {
		t.Fatalf("qolgan sanalar karta ichida qolishi kerak, takliflar: %d", first.OffersCount)
	}
}

// Aniq sana tanlanganda eski qoida saqlanadi: sana boshqa bo'lsa — boshqa tur.
func TestExactDateKeepsSeparateCards(t *testing.T) {
	tickets := []*models.Ticket{
		offerTicket(1, "malva_tour", 77, 500, "20261111", 7, 1200),
		offerTicket(2, "malva_tour", 77, 500, "20261112", 7, 1000),
	}

	if out := GroupTicketsByHotel(tickets, false); len(out) != 2 {
		t.Fatalf("ikki sana ikki karta bo'lishi kerak, chiqdi: %d", len(out))
	}
}

func TestDateRangeDetection(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]string
		want   bool
	}{
		{"oraliq", map[string]string{
			"CHECKIN_BEG": "20261110", "CHECKIN_END": "20261116",
			"NIGHTS_LIST": "7"}, true},
		{"aniq sana va kecha", map[string]string{
			"CHECKIN_BEG": "20261110", "CHECKIN_END": "20261110",
			"NIGHTS_LIST": "7"}, false},
		{"kecha berilmagan", map[string]string{
			"CHECKIN_BEG": "20261110", "CHECKIN_END": "20261110"}, true},
	}
	for _, tc := range cases {
		if got := dateRangeSearch(tc.params); got != tc.want {
			t.Errorf("%s: kutilgan %v, chiqdi %v", tc.name, tc.want, got)
		}
	}
}
