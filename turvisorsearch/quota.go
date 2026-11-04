package turvisorsearch

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tourvisor kuniga 3000 ta pullik so'rov beradi (qidiruv boshlash, continue,
// flights). Limit Moskva vaqti bilan 00:00 da yangilanadi. Hamma qidiruvlar
// shu yerdan ruxsat oladi: Redis'dagi hisob restartdan keyin ham saqlanadi.
// Hisob ishlamasa qidiruv qilinmaydi — limitni bilmasdan sarflamaymiz.

// Store — Redis ustidagi kichik interfeys (cache.RedisCache uni bajaradi).
type Store interface {
	IncrBy(ctx context.Context, key string, n int64, ttl time.Duration) (int64, error)
	GetRaw(ctx context.Context, key string) ([]byte, bool, error)
	SetRaw(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

type Purpose string

const (
	PurposeLive     Purpose = "live"
	PurposeCalendar Purpose = "calendar"

	// Django ham tur reyslarini (/flights, pullik) so'raydi va bu hisobga
	// kirmaydi — shuning uchun to'liq 3000 emas.
	defaultDailyLimit     = 2800
	defaultCalendarBudget = 600
	quotaKeyTTL           = 48 * time.Hour
)

var moscow = time.FixedZone("MSK", 3*60*60)

var (
	storeMu sync.RWMutex
	store   Store
)

// Configure main.go'da bir marta chaqiriladi.
func Configure(s Store) {
	storeMu.Lock()
	store = s
	storeMu.Unlock()
}

func currentStore() Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return store
}

func envInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func envString(name, def string) string {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		return raw
	}
	return def
}

func dailyLimit() int     { return envInt("TOURVISOR_DAILY_LIMIT", defaultDailyLimit) }
func calendarBudget() int { return envInt("TOURVISOR_CALENDAR_DAILY_BUDGET", defaultCalendarBudget) }

func quotaDay(now time.Time) string { return now.In(moscow).Format("20060102") }

func totalKey(day string) string    { return "tourvisor:billable:" + day }
func calendarKey(day string) string { return "tourvisor:billable:calendar:" + day }

// liveCap — jonli qidiruvlar kalendar ulushiga tegmaydi.
func liveCap(limit, calendar int) int {
	if calendar > limit {
		return 0
	}
	return limit - calendar
}

// reserveBillable bitta pullik so'rovga ruxsat so'raydi.
func reserveBillable(ctx context.Context, purpose Purpose) bool {
	s := currentStore()
	if s == nil {
		log.Printf("tourvisor quota store missing — %s search skipped", purpose)
		return false
	}
	day := quotaDay(time.Now())
	limit := dailyLimit()
	calendar := calendarBudget()

	total, err := s.IncrBy(ctx, totalKey(day), 1, quotaKeyTTL)
	if err != nil {
		log.Printf("tourvisor quota error=%v — %s search skipped", err, purpose)
		return false
	}
	release := func() {
		if _, err := s.IncrBy(context.Background(), totalKey(day), -1, quotaKeyTTL); err != nil {
			log.Printf("tourvisor quota release error=%v", err)
		}
	}

	switch purpose {
	case PurposeCalendar:
		if int(total) > limit {
			release()
			log.Printf("tourvisor quota reached total=%d limit=%d — calendar search skipped", total-1, limit)
			return false
		}
		used, err := s.IncrBy(ctx, calendarKey(day), 1, quotaKeyTTL)
		if err != nil || int(used) > calendar {
			release()
			if err == nil {
				_, _ = s.IncrBy(context.Background(), calendarKey(day), -1, quotaKeyTTL)
			}
			log.Printf("tourvisor calendar budget reached used=%d budget=%d err=%v", used-1, calendar, err)
			return false
		}
	default:
		if int(total) > liveCap(limit, calendar) {
			release()
			log.Printf("tourvisor quota reached total=%d live cap=%d — live search skipped", total-1, liveCap(limit, calendar))
			return false
		}
	}
	return true
}
