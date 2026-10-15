package handlers

import (
	"testing"

	"go-operator-service/models"
)

func TestSliceHotToursSAMORestKeepsAll(t *testing.T) {
	cached := hotToursFixture(6)
	out := sliceHotTours(cached, "rest", 1, false)
	if got := len(out.Data.Results.Tickets); got != 6 {
		t.Fatalf("samo rest tickets = %d, want 6", got)
	}
}

func TestSliceHotToursTourvisorRestSkipsFeatured(t *testing.T) {
	cached := hotToursFixture(6)
	out := sliceHotTours(cached, "rest", 1, true)
	if got := len(out.Data.Results.Tickets); got != 2 {
		t.Fatalf("tourvisor rest tickets = %d, want 2", got)
	}
	if out.Data.Results.Tickets[0].ID != 5 {
		t.Fatalf("first rest id = %d, want 5", out.Data.Results.Tickets[0].ID)
	}
}

func TestSliceHotToursFeatured(t *testing.T) {
	cached := hotToursFixture(6)
	out := sliceHotTours(cached, "featured", 1, false)
	if got := len(out.Data.Results.Tickets); got != 4 {
		t.Fatalf("featured tickets = %d, want 4", got)
	}
}

func hotToursFixture(n int) *models.AsyncSamoResult {
	tickets := make([]*models.Ticket, n)
	for i := 0; i < n; i++ {
		tickets[i] = &models.Ticket{ID: i + 1}
	}
	return &models.AsyncSamoResult{
		Data: models.AsyncSamoData{
			Results: models.AsyncSamoResultPayload{Tickets: tickets},
		},
	}
}
