package turvisorsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Jonli qidiruvda Tourvisor'dan faqat biz to'g'ridan-to'g'ri ulanmagan
// operatorlar olinadi — ulanganlarining turlari SAMO orqali allaqachon bor.
// Tourvisor ID'lari (Toshkent, /operators?departureId=106, 2026-11-04):
//
//	90  Kompas (KZ)        — samo_tour (online.uz.kompastour.com)
//	180 Easy Booking (UZ)  — easy_booking
//	192 Aqua Travel Group  — aqua_travelplus
//	198 Malva Tour (UZ)    — malva_tour
//	204 FlyKhiva Travel    — flykhiva
const defaultExcludedOperatorIDs = "90,180,192,198,204"

const (
	operatorsURL      = "https://api.tourvisor.ru/search/api/v1/operators"
	operatorsCacheTTL = 6 * time.Hour
	operatorsRetry    = 5 * time.Minute
)

type operatorsEntry struct {
	ids     []int
	fetched time.Time
	failed  bool
}

var (
	operatorsMu    sync.Mutex
	operatorsCache = map[int]operatorsEntry{}
)

func excludedOperatorIDs() map[int]bool {
	// Bo'sh qiymat ham standart ro'yxatni beradi: xato .env ulangan
	// operatorlarni ikki marta chiqarib yubormasin.
	raw := envString("TOURVISOR_EXCLUDE_OPERATOR_IDS", defaultExcludedOperatorIDs)
	out := map[int]bool{}
	for _, part := range splitCSV(raw) {
		if id := atoi(part); id > 0 {
			out[id] = true
		}
	}
	return out
}

// filterOperators ulangan operatorlarni olib tashlaydi va tartiblaydi.
func filterOperators(all []int, excluded map[int]bool) []int {
	out := make([]int, 0, len(all))
	seen := map[int]bool{}
	for _, id := range all {
		if id <= 0 || excluded[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// liveOperatorIDs — jonli qidiruvga ruxsat etilgan operatorlar.
// Ro'yxat olinmasa bo'sh qaytadi va qidiruv qilinmaydi: filtrsiz qidirsak
// ulangan operatorlarning turlari ikki marta chiqadi.
func liveOperatorIDs(ctx context.Context, departureID int) ([]int, error) {
	all, err := departureOperators(ctx, departureID)
	if err != nil {
		return nil, err
	}
	return filterOperators(all, excludedOperatorIDs()), nil
}

// departureOperators /operators bepul ma'lumotnoma; 6 soat xotirada turadi.
func departureOperators(ctx context.Context, departureID int) ([]int, error) {
	operatorsMu.Lock()
	entry, ok := operatorsCache[departureID]
	operatorsMu.Unlock()
	if ok {
		age := time.Since(entry.fetched)
		if !entry.failed && age < operatorsCacheTTL {
			return entry.ids, nil
		}
		if entry.failed && age < operatorsRetry {
			if len(entry.ids) > 0 {
				return entry.ids, nil
			}
			return nil, fmt.Errorf("tourvisor operators unavailable")
		}
	}

	ids, err := fetchOperators(ctx, departureID)
	operatorsMu.Lock()
	defer operatorsMu.Unlock()
	if err != nil {
		// Eski ro'yxat bo'lsa, shuni ishlatamiz.
		operatorsCache[departureID] = operatorsEntry{ids: entry.ids, fetched: time.Now(), failed: true}
		if len(entry.ids) > 0 {
			log.Printf("tourvisor operators refresh failed departureId=%d err=%v — using previous list", departureID, err)
			return entry.ids, nil
		}
		return nil, err
	}
	operatorsCache[departureID] = operatorsEntry{ids: ids, fetched: time.Now()}
	return ids, nil
}

func fetchOperators(ctx context.Context, departureID int) ([]int, error) {
	values := url.Values{}
	values.Set("departureId", strconv.Itoa(departureID))
	body, status, err := getJSON(ctx, operatorsURL+"?"+values.Encode())
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("tourvisor operators status %d", status)
	}
	return decodeOperatorIDs(body)
}

func decodeOperatorIDs(body []byte) ([]int, error) {
	var items []struct {
		ID int `json:"id"`
	}
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") {
		var wrapped struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrapped); err != nil {
			return nil, err
		}
		trimmed = string(wrapped.Data)
	}
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(items))
	for _, item := range items {
		if item.ID > 0 {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("tourvisor operators list is empty")
	}
	return ids, nil
}
