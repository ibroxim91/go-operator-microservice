package turvisorsearch

import (
	"context"
	"testing"
	"time"
)

func day(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

func TestCalendarWindowsSkipEmptyWeeks(t *testing.T) {
	dates := []string{"2026-11-04", "2026-11-05", "2026-11-07", "2026-11-12", "2026-11-30", "2027-03-01", "bad"}
	got := calendarWindows(dates, day("2026-11-05"), day("2026-12-03"), 7)
	want := [][2]string{
		{"2026-11-05", "2026-11-11"},
		{"2026-11-12", "2026-11-18"},
		{"2026-11-30", "2026-12-03"}, // oxiri gorizontda kesiladi
	}
	if len(got) != len(want) {
		t.Fatalf("windows = %v", got)
	}
	for i, w := range want {
		if got[i].from.Format("2006-01-02") != w[0] || got[i].to.Format("2006-01-02") != w[1] {
			t.Fatalf("window %d = %s..%s want %v", i, got[i].from.Format("2006-01-02"), got[i].to.Format("2006-01-02"), w)
		}
		if got[i].to.Sub(got[i].from) > 20*24*time.Hour {
			t.Fatal("window over Tourvisor 21-day limit")
		}
	}
}

func TestCalendarWindowsNoDatesNoSearch(t *testing.T) {
	if len(calendarWindows(nil, day("2026-11-05"), day("2027-02-02"), 7)) != 0 {
		t.Fatal("no dates → no billable search")
	}
}

func TestMinPricesByDateOnlySellable(t *testing.T) {
	hotels := []searchHotel{
		{ID: 1, Tours: []searchTour{
			{Date: "2026-11-25", Price: 1100, IsCharter: boolPtr(true)},
			{Date: "2026-11-25", Price: 700, IsCharter: boolPtr(false)},             // oddiy reys — hisobga olinmaydi
			{Date: "2026-11-25", Price: 650, IsCharter: boolPtr(true), Name: "GDS"}, // GDS — hisobga olinmaydi
		}},
		{ID: 2, Tours: []searchTour{
			{Date: "2026-11-25", Price: 972, IsCharter: boolPtr(true)},
			{Date: "2026-11-26", Price: 1093, IsCharter: boolPtr(true)},
		}},
	}
	got := minPricesByDate(hotels)
	if got["2026-11-25"] != 972 || got["2026-11-26"] != 1093 || len(got) != 2 {
		t.Fatalf("prices = %v", got)
	}
}

func TestCarryOverKeepsOldPricesOfFailedWindow(t *testing.T) {
	prev := &Calendar{PricesUSD: map[string]float64{"2026-11-10": 900, "2026-11-20": 800}}
	dst := map[string]float64{}
	carryOver(dst, prev, calendarWindow{from: day("2026-11-08"), to: day("2026-11-14")})
	if dst["2026-11-10"] != 900 || len(dst) != 1 {
		t.Fatalf("carried = %v", dst)
	}
	carryOver(dst, nil, calendarWindow{from: day("2026-11-08"), to: day("2026-11-14")})
}

func TestCalendarSaveLoad(t *testing.T) {
	withStore(t, newFakeStore())
	ctx := context.Background()
	cal := &Calendar{DepartureID: 21, CountryID: 25, Adults: 2, PricesUSD: map[string]float64{"2026-11-25": 972}}
	if err := saveCalendar(ctx, cal); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadCalendar(ctx, 21, 25)
	if err != nil || !ok || got.PricesUSD["2026-11-25"] != 972 {
		t.Fatalf("loaded = %#v %v %v", got, ok, err)
	}
	if _, ok, _ := LoadCalendar(ctx, 21, 99); ok {
		t.Fatal("unknown route must miss")
	}
}

func TestCalendarSearchHasNoOperatorFilter(t *testing.T) {
	// Kalendar barcha operatorlarning (bizning SAMO'lar ham) eng arzon charter narxini oladi.
	q := Query{DepartureID: 106, CountryID: 9, Adults: calendarAdults, NightsFrom: calendarNightsFrom, NightsTo: calendarNightsTo,
		DateFrom: day("2026-11-05"), DateTo: day("2026-11-11")}
	v := q.Values()
	if _, ok := v["operatorIds"]; ok {
		t.Fatal("calendar query must not filter operators")
	}
	if v.Get("onlyCharter") != "true" || v.Get("nightsFrom") != "7" || v.Get("nightsTo") != "14" || v.Get("adults") != "2" {
		t.Fatalf("calendar query = %s", v.Encode())
	}
}

func TestKeepOutsideLeavesOtherPassAndDropsPast(t *testing.T) {
	prev := &Calendar{PricesUSD: map[string]float64{
		"2026-11-03": 500, // o'tgan kun — tashlanadi
		"2026-11-10": 900, // shu bosqich ichida — yangisi yoziladi
		"2027-01-20": 800, // 61–90 bosqichi — qoladi
	}}
	got := keepOutside(prev, day("2026-11-05"), day("2026-11-05"), day("2027-01-03"))
	if len(got) != 1 || got["2027-01-20"] != 800 {
		t.Fatalf("kept = %v", got)
	}
	if len(keepOutside(nil, day("2026-11-05"), day("2026-11-05"), day("2027-01-03"))) != 0 {
		t.Fatal("nil prev")
	}
}

func TestCalendarPassesCoverHorizonWithoutGap(t *testing.T) {
	if calendarNearDays <= 0 || calendarNearDays >= calendarHorizonDays {
		t.Fatalf("near=%d horizon=%d", calendarNearDays, calendarHorizonDays)
	}
	tomorrow := day("2026-11-05")
	nearEnd := tomorrow.AddDate(0, 0, calendarNearDays-1)
	farStart := tomorrow.AddDate(0, 0, calendarNearDays)
	if farStart.Sub(nearEnd) != 24*time.Hour {
		t.Fatal("passes must touch without gap or overlap")
	}
}
