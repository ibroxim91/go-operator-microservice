package turvisorsearch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu      sync.Mutex
	counts  map[string]int64
	values  map[string][]byte
	failInc bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{counts: map[string]int64{}, values: map[string][]byte{}}
}

func (f *fakeStore) IncrBy(_ context.Context, key string, n int64, _ time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failInc {
		return 0, errors.New("redis down")
	}
	f.counts[key] += n
	return f.counts[key], nil
}

func (f *fakeStore) GetRaw(_ context.Context, key string) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeStore) SetRaw(_ context.Context, key string, value []byte, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = value
	return nil
}

func withStore(t *testing.T, s Store) {
	t.Helper()
	Configure(s)
	t.Cleanup(func() { Configure(nil) })
}

func boolPtr(v bool) *bool { return &v }

func TestIsSellableTourDropsGDSAndRegularFlights(t *testing.T) {
	cases := []struct {
		name string
		tour searchTour
		want bool
	}{
		{"charter", searchTour{Name: "Отдых в ОАЭ", Price: 900, IsCharter: boolPtr(true)}, true},
		{"regular flight", searchTour{Name: "Отдых в ОАЭ", Price: 900, IsCharter: boolPtr(false)}, false},
		{"unknown flight", searchTour{Name: "Отдых в ОАЭ", Price: 900}, false},
		{"gds name", searchTour{Name: "GDS Dubai", Price: 900, IsCharter: boolPtr(true)}, false},
		{"gds cyrillic", searchTour{Name: "Тур ГДС", Price: 900, IsCharter: boolPtr(true)}, false},
		{"no price", searchTour{Name: "Отдых", Price: 0, IsCharter: boolPtr(true)}, false},
	}
	for _, tc := range cases {
		if got := isSellableTour(tc.tour); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestMapSearchTourSkipsRegularFlight(t *testing.T) {
	hotel := searchHotel{ID: 5, Name: "Hotel"}
	tour := searchTour{ID: "1", Date: "2026-11-20", Nights: 7, Price: 900, IsCharter: boolPtr(false)}
	if mapSearchTour(hotel, tour, Query{Adults: 2}, 12000) != nil {
		t.Fatal("regular-flight tour must not become a ticket")
	}
}

func TestValuesAlwaysCharterAndRepeatsOperators(t *testing.T) {
	q := Query{
		DepartureID: 106, CountryID: 9, Adults: 2, NightsFrom: 7, NightsTo: 14,
		DateFrom:    time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
		DateTo:      time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
		OperatorIDs: []int{12, 93, 179},
	}
	v := q.Values()
	if v.Get("onlyCharter") != "true" {
		t.Fatal("onlyCharter must always be true")
	}
	if got := strings.Join(v["operatorIds"], ","); got != "12,93,179" {
		t.Fatalf("operatorIds = %v", v["operatorIds"])
	}
	if _, ok := v["childs"]; ok {
		t.Fatal("no children → no childs param")
	}
}

func TestStatusURLHasRequiredOperatorStatus(t *testing.T) {
	if !strings.HasSuffix(statusURL("42"), "/42/status?operatorStatus=false") {
		t.Fatalf("status url = %s", statusURL("42"))
	}
}

func TestFilterOperatorsDropsIntegrated(t *testing.T) {
	t.Setenv("TOURVISOR_EXCLUDE_OPERATOR_IDS", defaultExcludedOperatorIDs)
	all := []int{12, 90, 93, 179, 180, 188, 192, 194, 198, 204, 93}
	got := filterOperators(all, excludedOperatorIDs())
	want := []int{12, 93, 179, 188, 194}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestExcludedOperatorsDefaultWhenUnset(t *testing.T) {
	ex := excludedOperatorIDs()
	for _, id := range []int{90, 180, 192, 198, 204} {
		if !ex[id] {
			t.Fatalf("operator %d must be excluded by default", id)
		}
	}
}

func TestDecodeOperatorIDs(t *testing.T) {
	ids, err := decodeOperatorIDs([]byte(`[{"id":12,"name":"Pegas"},{"id":90,"name":"Kompas"}]`))
	if err != nil || len(ids) != 2 || ids[0] != 12 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if _, err := decodeOperatorIDs([]byte(`[]`)); err == nil {
		t.Fatal("empty list must be an error — search would run unfiltered")
	}
}

func TestQuotaLiveLeavesCalendarShare(t *testing.T) {
	s := newFakeStore()
	withStore(t, s)
	t.Setenv("TOURVISOR_DAILY_LIMIT", "10")
	t.Setenv("TOURVISOR_CALENDAR_DAILY_BUDGET", "4")
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		if !reserveBillable(ctx, PurposeLive) {
			t.Fatalf("live %d should pass", i+1)
		}
	}
	if reserveBillable(ctx, PurposeLive) {
		t.Fatal("7th live must be refused (4 kept for calendar)")
	}
	for i := 0; i < 4; i++ {
		if !reserveBillable(ctx, PurposeCalendar) {
			t.Fatalf("calendar %d should pass", i+1)
		}
	}
	if reserveBillable(ctx, PurposeCalendar) {
		t.Fatal("calendar over budget must be refused")
	}
	day := quotaDay(time.Now())
	if s.counts[totalKey(day)] != 10 {
		t.Fatalf("total = %d, refused calls must be rolled back", s.counts[totalKey(day)])
	}
}

func TestQuotaFailsClosed(t *testing.T) {
	Configure(nil)
	if reserveBillable(context.Background(), PurposeLive) {
		t.Fatal("no store → no search")
	}
	s := newFakeStore()
	s.failInc = true
	withStore(t, s)
	if reserveBillable(context.Background(), PurposeLive) {
		t.Fatal("redis error → no search")
	}
}

func TestQuotaDayIsMoscow(t *testing.T) {
	// 22:30 UTC = ertasi kun 01:30 Moskva.
	now := time.Date(2026, 11, 4, 22, 30, 0, 0, time.UTC)
	if quotaDay(now) != "20261105" {
		t.Fatalf("day = %s", quotaDay(now))
	}
}

func TestResultsCacheRoundTrip(t *testing.T) {
	withStore(t, newFakeStore())
	q := Query{DepartureID: 106, CountryID: 9, Adults: 2, OperatorIDs: []int{12}}
	key := resultsCacheKey(q)
	if key != resultsCacheKey(q) {
		t.Fatal("key must be stable")
	}
	q2 := q
	q2.Adults = 3
	if key == resultsCacheKey(q2) {
		t.Fatal("different query must have a different key")
	}
	hotels := []searchHotel{{ID: 5, Name: "H", Tours: []searchTour{{ID: "7", Price: 900, IsCharter: boolPtr(true)}}}}
	saveCachedHotels(key, hotels, true)
	got, ok := loadCachedHotels(context.Background(), key)
	if !ok || len(got) != 1 || got[0].Tours[0].tourID() != "7" || !isSellableTour(got[0].Tours[0]) {
		t.Fatalf("cached = %#v %v", got, ok)
	}
}
