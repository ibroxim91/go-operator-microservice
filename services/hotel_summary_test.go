package services

import (
	"testing"

	"go-operator-service/models"
)

func summaryTicket(operator string, hotelDBID, operatorHotelID int, name string) *models.Ticket {
	return &models.Ticket{
		Operator:    operator,
		HotelDBID:   hotelDBID,
		TicketHotel: []models.TicketHotel{{ID: operatorHotelID, Name: name}},
	}
}

// Filtr bizning katalogdagi id'ni bilishi kerak: operator raqami faqat
// bitta operatorni biladi, bizning id esa hammasini.
func TestHotelSummaryCarriesCatalogueID(t *testing.T) {
	out := BuildHotelSummaries([]*models.Ticket{
		summaryTicket("samo_tour", 55, 900, "ADORA HOTEL ANTALYA"),
	})

	if len(out) != 1 {
		t.Fatalf("bitta qator kutilgandi, chiqdi: %d", len(out))
	}
	if out[0].HotelDBID != 55 {
		t.Fatalf("hotel_db_id yuborilishi kerak, chiqdi: %d", out[0].HotelDBID)
	}
	if out[0].ID != 900 {
		t.Fatalf("operator id o'zgarmasligi kerak, chiqdi: %d", out[0].ID)
	}
}

// Mapping yo'q mehmonxonada maydon bo'sh qoladi va javobda umuman
// ko'rinmaydi (omitempty).
func TestHotelSummaryWithoutMapping(t *testing.T) {
	out := BuildHotelSummaries([]*models.Ticket{
		summaryTicket("samo_tour", 0, 900, "WHITE FORT"),
	})

	if len(out) != 1 || out[0].HotelDBID != 0 {
		t.Fatalf("mapping yo'q qatorda hotel_db_id bo'lmasligi kerak: %+v", out)
	}
}

// Eski xulq saqlanadi: har operator o'z qatorida qoladi.
func TestHotelSummaryKeepsOneRowPerOperator(t *testing.T) {
	out := BuildHotelSummaries([]*models.Ticket{
		summaryTicket("samo_tour", 55, 900, "ADORA HOTEL ANTALYA"),
		summaryTicket("malva_tour", 55, 701, "Adora Hotel"),
	})

	if len(out) != 2 {
		t.Fatalf("ikki operator ikki qator bo'lishi kerak, chiqdi: %d", len(out))
	}
}
