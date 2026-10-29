package handlers

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-operator-service/models"
	"go-operator-service/services"
)

func sampleResult() *models.AsyncSamoResult {
	return &models.AsyncSamoResult{
		Status: true,
		Data: models.AsyncSamoData{
			Results: models.AsyncSamoResultPayload{
				Tickets: []*models.Ticket{{ID: 1, Slug: "tour-a"}, {ID: 2, Slug: "tour-b"}},
			},
		},
	}
}

func TestIdenticalSearchesReachOperatorsOnce(t *testing.T) {
	var loads int32
	release := make(chan struct{})
	load := func() (*models.AsyncSamoResult, error) {
		atomic.AddInt32(&loads, 1)
		<-release
		return sampleResult(), nil
	}

	const callers = 20
	var wg sync.WaitGroup
	var sharedCount int32
	results := make([]*models.AsyncSamoResult, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, shared, err := coalesceSearch("uae", load)
			if err != nil {
				t.Error(err)
			}
			if shared {
				atomic.AddInt32(&sharedCount, 1)
			}
			results[i] = res
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if loads != 1 {
		t.Fatalf("operatorlarga %d marta borildi, 1 marta kutilgan", loads)
	}
	if sharedCount != callers-1 {
		t.Fatalf("birlashgan chaqiruvlar %d, %d kutilgan", sharedCount, callers-1)
	}
	for i, res := range results {
		if res == nil || len(res.Data.Results.Tickets) != 2 {
			t.Fatalf("%d-chaqiruv natijasiz qoldi", i)
		}
	}
}

func TestEachCallerGetsItsOwnCopy(t *testing.T) {
	original := sampleResult()
	release := make(chan struct{})
	load := func() (*models.AsyncSamoResult, error) { <-release; return original, nil }

	var first, second *models.AsyncSamoResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); first, _, _ = coalesceSearch("copy", load) }()
	time.Sleep(20 * time.Millisecond)
	go func() { defer wg.Done(); second, _, _ = coalesceSearch("copy", load) }()
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	// sahifalash shunday qiladi: share token, slug, tartib
	first.Data.Results.Tickets[0].Slug = "changed"
	first.Data.Results.Tickets[0], first.Data.Results.Tickets[1] = first.Data.Results.Tickets[1], first.Data.Results.Tickets[0]

	if second.Data.Results.Tickets[0].Slug != "tour-a" {
		t.Fatal("bir mijozning o'zgarishi boshqasiga o'tdi")
	}
	if original.Data.Results.Tickets[0].Slug != "tour-a" || original.Data.Results.Tickets[0].ID != 1 {
		t.Fatal("asl natija o'zgardi")
	}
}

func TestDifferentSearchesRunSeparately(t *testing.T) {
	var loads int32
	load := func() (*models.AsyncSamoResult, error) {
		atomic.AddInt32(&loads, 1)
		return sampleResult(), nil
	}
	_, _, _ = coalesceSearch("uae", load)
	_, _, _ = coalesceSearch("turkey", load)
	_, _, _ = coalesceSearch("uae", load) // birinchisi tugagan — yangi qidiruv
	if loads != 3 {
		t.Fatalf("%d yuklash, 3 kutilgan", loads)
	}
}

func TestErrorReachesEveryWaiter(t *testing.T) {
	boom := errors.New("operators down")
	release := make(chan struct{})
	load := func() (*models.AsyncSamoResult, error) { <-release; return nil, boom }
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, _, errs[i] = coalesceSearch("err", load) }(i)
	}
	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, boom) {
			t.Fatalf("%d-chaqiruv xatoni olmadi: %v", i, err)
		}
	}
}

func TestPanicDoesNotLeaveWaitersHanging(t *testing.T) {
	release := make(chan struct{})
	waiterDone := make(chan struct{})
	go func() {
		defer func() { _ = recover() }()
		_, _, _ = coalesceSearch("panic", func() (*models.AsyncSamoResult, error) { <-release; panic("boom") })
	}()
	time.Sleep(20 * time.Millisecond)
	go func() {
		_, _, _ = coalesceSearch("panic", func() (*models.AsyncSamoResult, error) { return sampleResult(), nil })
		close(waiterDone)
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	select {
	case <-waiterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("panikadan keyin kutayotgan chaqiruv qotib qoldi")
	}
}

func TestPaginateReportsTotalOffersAcrossAllPages(t *testing.T) {
	// 75 mehmonxona, har birida 2 taklif — guruhlash 75 karta qiladi
	tickets := make([]*models.Ticket, 0, 150)
	for i := 0; i < 150; i++ {
		tickets = append(tickets, &models.Ticket{
			ID: i + 1, HotelDBID: 1000 + i/2, DepartureDate: "20261105", Nights: 7,
			PriceFull: 100 + i, Operator: "samo_tour",
		})
	}
	res := &models.AsyncSamoResult{Data: models.AsyncSamoData{Results: models.AsyncSamoResultPayload{Tickets: tickets}}}
	page := paginateAsyncSamoResult(context.Background(), nil, res, 1, services.TicketSortMode{SkipRecommendedInterleave: true})
	if page.Data.TotalItems != 75 || page.Data.TotalOffers != 150 {
		t.Fatalf("total_items=%d total_offers=%d, 75/150 kutilgan", page.Data.TotalItems, page.Data.TotalOffers)
	}
}
