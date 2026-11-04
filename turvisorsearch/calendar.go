package turvisorsearch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-operator-service/repository"
)

// Kalendar narxlari: har (jo'nash, davlat) uchun keyingi kunlarning eng
// arzon charter narxi (2 katta, 7–14 kecha — qidiruvning odatiy oralig'i).
// Fonda kuniga bir marta yig'iladi; foydalanuvchi so'rovi hech qachon
// pullik qidiruv boshlamaydi — faqat Redis'dan o'qiydi.

const (
	calendarDatesURL     = "https://api.tourvisor.ru/search/api/v1/tours/dates"
	calendarAdults       = 2
	calendarNightsFrom   = 7
	calendarNightsTo     = 14
	calendarWindowDays   = 7 // Tourvisor 21 kungacha ruxsat beradi; 21 kunda kunlarning ~40% bo'sh qoldi
	calendarHorizonDays  = 90
	calendarResultsLimit = 300
	calendarPollEvery    = 5 * time.Second
	calendarSearchMax    = 4 * time.Minute
	calendarWorkers      = 4
	calendarTTL          = 72 * time.Hour
	calendarKeyPrefix    = "tourvisor:calendar:v1:"
)

// Avval mashhur yo'nalishlar: limit tugasa, chetdagilari qoladi.
// Tourvisor davlat ID'lari: Turkiya, BAA, Misr, Tailand, Vyetnam, Maldiv, Xitoy, Malayziya, Shri-Lanka, Indoneziya.
var calendarCountryPriority = []int{4, 9, 1, 2, 16, 8, 13, 36, 12, 7}

// Calendar — Redis'da saqlanadigan ko'rinish.
type Calendar struct {
	DepartureID int                `json:"departure_id"`
	CountryID   int                `json:"country_id"`
	Adults      int                `json:"adults"`
	NightsFrom  int                `json:"nights_from"`
	NightsTo    int                `json:"nights_to"`
	UpdatedAt   time.Time          `json:"updated_at"`
	PricesUSD   map[string]float64 `json:"prices_usd"` // "2026-11-25" -> 972 (jami, 2 kishi)
}

type calendarRoute struct {
	ourDepartureID int
	ourCountryID   int
	tvDepartureID  int
	tvCountryID    int
}

type calendarWindow struct {
	from, to time.Time
}

func CalendarKey(ourDepartureID, ourCountryID int) string {
	return calendarKeyPrefix + strconv.Itoa(ourDepartureID) + ":" + strconv.Itoa(ourCountryID)
}

// LoadCalendar — endpoint uchun; topilmasa ok=false.
func LoadCalendar(ctx context.Context, ourDepartureID, ourCountryID int) (*Calendar, bool, error) {
	s := currentStore()
	if s == nil {
		return nil, false, fmt.Errorf("tourvisor store missing")
	}
	raw, ok, err := s.GetRaw(ctx, CalendarKey(ourDepartureID, ourCountryID))
	if err != nil || !ok {
		return nil, false, err
	}
	var cal Calendar
	if err := json.Unmarshal(raw, &cal); err != nil {
		return nil, false, err
	}
	return &cal, true, nil
}

func saveCalendar(ctx context.Context, cal *Calendar) error {
	s := currentStore()
	if s == nil {
		return fmt.Errorf("tourvisor store missing")
	}
	raw, err := json.Marshal(cal)
	if err != nil {
		return err
	}
	return s.SetRaw(ctx, CalendarKey(cal.DepartureID, cal.CountryID), raw, calendarTTL)
}

// calendarDepartures — bizning jo'nash ID'lari (TOURVISOR_CALENDAR_DEPARTURES, standart: 21 — Toshkent).
func calendarDepartures() []int {
	raw := envString("TOURVISOR_CALENDAR_DEPARTURES", "21")
	out := []int{}
	for _, part := range splitCSV(raw) {
		if id := atoi(part); id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func calendarRoutes(db *sql.DB) ([]calendarRoute, error) {
	rows, err := db.Query(`SELECT country_id, operator_state_id FROM api_countrymapping WHERE operator = $1`, operatorName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type country struct{ our, tv int }
	countries := []country{}
	for rows.Next() {
		var our int
		var tvRaw string
		if err := rows.Scan(&our, &tvRaw); err != nil {
			return nil, err
		}
		if tv := atoi(tvRaw); our > 0 && tv > 0 {
			countries = append(countries, country{our: our, tv: tv})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rank := map[int]int{}
	for i, id := range calendarCountryPriority {
		rank[id] = i + 1
	}
	sort.SliceStable(countries, func(i, j int) bool {
		ri, rj := rank[countries[i].tv], rank[countries[j].tv]
		if ri == 0 {
			ri = len(calendarCountryPriority) + 1
		}
		if rj == 0 {
			rj = len(calendarCountryPriority) + 1
		}
		if ri != rj {
			return ri < rj
		}
		return countries[i].tv < countries[j].tv
	})

	// TOURVISOR_CALENDAR_COUNTRY_IDS — bizning davlat ID'lari; bo'sh bo'lsa hammasi.
	only := map[int]bool{}
	for _, part := range splitCSV(envString("TOURVISOR_CALENDAR_COUNTRY_IDS", "")) {
		if id := atoi(part); id > 0 {
			only[id] = true
		}
	}

	routes := []calendarRoute{}
	for _, dep := range calendarDepartures() {
		mapping, err := repository.GetRegionMapping(db, operatorName, dep)
		if err != nil || mapping == nil || mapping.OperatorTownID <= 0 {
			log.Printf("tourvisor calendar: no departure mapping for %d err=%v", dep, err)
			continue
		}
		for _, c := range countries {
			if len(only) > 0 && !only[c.our] {
				continue
			}
			routes = append(routes, calendarRoute{
				ourDepartureID: dep,
				ourCountryID:   c.our,
				tvDepartureID:  mapping.OperatorTownID,
				tvCountryID:    c.tv,
			})
		}
	}
	return routes, nil
}

// RefreshCalendars hamma yo'nalishni yangilaydi. Xato bo'lgan oyna uchun
// eski narxlar saqlanib qoladi — kalendar hech qachon to'satdan bo'shamaydi.
func RefreshCalendars(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	routes, err := calendarRoutes(db)
	if err != nil {
		log.Printf("tourvisor calendar routes failed err=%v", err)
		return
	}
	started := time.Now()
	jobs := make(chan calendarRoute)
	var wg sync.WaitGroup
	var mu sync.Mutex
	searches, saved := 0, 0
	for i := 0; i < calendarWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for route := range jobs {
				n, ok := refreshRoute(ctx, route)
				mu.Lock()
				searches += n
				if ok {
					saved++
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for _, route := range routes {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- route:
		}
	}
	close(jobs)
	wg.Wait()
	log.Printf("tourvisor calendar refresh done routes=%d saved=%d searches=%d took=%s", len(routes), saved, searches, time.Since(started).Round(time.Second))
}

func refreshRoute(ctx context.Context, route calendarRoute) (int, bool) {
	today := time.Now().In(moscow)
	start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	end := start.AddDate(0, 0, calendarHorizonDays-1)

	dates, err := fetchTourDates(ctx, route.tvDepartureID, route.tvCountryID)
	if err != nil {
		log.Printf("tourvisor calendar dates failed dep=%d country=%d err=%v", route.tvDepartureID, route.tvCountryID, err)
		return 0, false
	}
	windows := calendarWindows(dates, start, end, calendarWindowDays)
	if len(windows) == 0 {
		return 0, false
	}

	prev, _, _ := LoadCalendar(ctx, route.ourDepartureID, route.ourCountryID)
	prices := map[string]float64{}
	searches := 0
	for _, w := range windows {
		if ctx.Err() != nil {
			break
		}
		got, billed, err := searchWindow(ctx, route, w)
		if billed {
			searches++
		}
		if err != nil {
			log.Printf("tourvisor calendar window failed dep=%d country=%d %s..%s err=%v",
				route.tvDepartureID, route.tvCountryID, w.from.Format("2006-01-02"), w.to.Format("2006-01-02"), err)
			carryOver(prices, prev, w)
			continue
		}
		for date, price := range got {
			prices[date] = price
		}
	}
	if len(prices) == 0 {
		return searches, false
	}
	cal := &Calendar{
		DepartureID: route.ourDepartureID,
		CountryID:   route.ourCountryID,
		Adults:      calendarAdults,
		NightsFrom:  calendarNightsFrom,
		NightsTo:    calendarNightsTo,
		UpdatedAt:   time.Now().UTC(),
		PricesUSD:   prices,
	}
	if err := saveCalendar(ctx, cal); err != nil {
		log.Printf("tourvisor calendar save failed err=%v", err)
		return searches, false
	}
	return searches, true
}

// carryOver — oyna yangilanmasa, undagi eski narxlar qoladi.
func carryOver(dst map[string]float64, prev *Calendar, w calendarWindow) {
	if prev == nil {
		return
	}
	for date, price := range prev.PricesUSD {
		d, err := time.Parse("2006-01-02", date)
		if err != nil || d.Before(w.from) || d.After(w.to) {
			continue
		}
		dst[date] = price
	}
}

// calendarWindows — reys bor kunlarni 7 kunlik oynalarga yig'adi; reyssiz
// oynaga pullik qidiruv yuborilmaydi.
func calendarWindows(dates []string, start, end time.Time, size int) []calendarWindow {
	days := []time.Time{}
	for _, raw := range dates {
		d, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
		if err != nil || d.Before(start) || d.After(end) {
			continue
		}
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	windows := []calendarWindow{}
	for _, d := range days {
		if n := len(windows); n > 0 && !d.After(windows[n-1].to) {
			continue
		}
		to := d.AddDate(0, 0, size-1)
		if to.After(end) {
			to = end
		}
		windows = append(windows, calendarWindow{from: d, to: to})
	}
	return windows
}

func fetchTourDates(ctx context.Context, departureID, countryID int) ([]string, error) {
	values := url.Values{}
	values.Set("departureId", strconv.Itoa(departureID))
	values.Set("countryId", strconv.Itoa(countryID))
	values.Set("onlyCharter", "true")
	body, status, err := getJSON(ctx, calendarDatesURL+"?"+values.Encode())
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("tourvisor dates status %d", status)
	}
	var dates []string
	if err := json.Unmarshal(body, &dates); err != nil {
		return nil, err
	}
	return dates, nil
}

// searchWindow bitta pullik qidiruv; billed=false bo'lsa limit sarflanmagan.
func searchWindow(ctx context.Context, route calendarRoute, w calendarWindow) (map[string]float64, bool, error) {
	query := &Query{
		DepartureID: route.tvDepartureID,
		CountryID:   route.tvCountryID,
		DateFrom:    w.from,
		DateTo:      w.to,
		NightsFrom:  calendarNightsFrom,
		NightsTo:    calendarNightsTo,
		Adults:      calendarAdults,
	}
	if !reserveBillable(ctx, PurposeCalendar) {
		return nil, false, fmt.Errorf("quota")
	}
	searchID, err := startSearch(ctx, query)
	if err != nil {
		return nil, true, err
	}

	deadline := time.Now().Add(calendarSearchMax)
	done := false
	for time.Now().Before(deadline) {
		timer := time.NewTimer(calendarPollEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, true, ctx.Err()
		case <-timer.C:
		}
		body, status, pollErr := getJSON(ctx, statusURL(searchID))
		if pollErr == nil && status >= 200 && status < 300 && statusDone(decodeStatus(body)) {
			done = true
			break
		}
	}
	hotels, err := fetchResultsLimit(ctx, searchID, calendarResultsLimit)
	if err != nil {
		return nil, true, err
	}
	prices := minPricesByDate(hotels)
	if !done && len(prices) == 0 {
		return nil, true, fmt.Errorf("search not finished")
	}
	return prices, true, nil
}

// minPricesByDate — har sananing eng arzon sotiladigan (charter, GDS emas) narxi.
func minPricesByDate(hotels []searchHotel) map[string]float64 {
	out := map[string]float64{}
	for _, hotel := range hotels {
		for _, tour := range hotel.Tours {
			if !isSellableTour(tour) {
				continue
			}
			d, err := time.Parse("2006-01-02", strings.TrimSpace(tour.Date))
			if err != nil {
				continue
			}
			key := d.Format("2006-01-02")
			if cur, ok := out[key]; !ok || tour.Price < cur {
				out[key] = tour.Price
			}
		}
	}
	return out
}
