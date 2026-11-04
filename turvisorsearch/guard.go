package turvisorsearch

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"go-operator-service/models"
)

const (
	completeResultsTTL = 30 * time.Minute
	partialResultsTTL  = 10 * time.Minute
)

// isSellableTour — faqat charter reysli, GDS bo'lmagan va narxi bor tur.
// So'rovda onlyCharter=true yuboriladi; bu ikkinchi himoya. isCharter
// kelmagan tur ham tashlanadi — noaniq turni sotmaymiz.
func isSellableTour(tour searchTour) bool {
	if tour.IsCharter == nil || !*tour.IsCharter {
		return false
	}
	if models.IsGDSTour(tour.Name) {
		return false
	}
	return tour.Price > 0
}

func liveSearchWait() time.Duration {
	seconds := envInt("TOURVISOR_SEARCH_WAIT_SECONDS", int(searchWait/time.Second))
	if seconds < 5 || seconds > 60 {
		return searchWait
	}
	return time.Duration(seconds) * time.Second
}

// resultsCacheKey — bir xil qidiruv qayta pullik so'rov sarflamasligi uchun.
func resultsCacheKey(q Query) string {
	sum := sha1.Sum([]byte(q.Values().Encode()))
	return "tourvisor:results:v2:" + hex.EncodeToString(sum[:])
}

func loadCachedHotels(ctx context.Context, key string) ([]searchHotel, bool) {
	s := currentStore()
	if s == nil {
		return nil, false
	}
	raw, ok, err := s.GetRaw(ctx, key)
	if err != nil || !ok {
		return nil, false
	}
	var hotels []searchHotel
	if err := json.Unmarshal(raw, &hotels); err != nil {
		return nil, false
	}
	return hotels, true
}

// saveCachedHotels — xom natija saqlanadi, so'm narxi o'qishda hisoblanadi.
func saveCachedHotels(key string, hotels []searchHotel, complete bool) {
	s := currentStore()
	if s == nil || len(hotels) == 0 {
		return
	}
	raw, err := json.Marshal(hotels)
	if err != nil {
		return
	}
	ttl := partialResultsTTL
	if complete {
		ttl = completeResultsTTL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.SetRaw(ctx, key, raw, ttl); err != nil {
		log.Printf("tourvisor results cache save failed err=%v", err)
	}
}
