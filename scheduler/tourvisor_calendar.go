package scheduler

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/turvisorsearch"
)

const (
	tourvisorCalendarLastRunKey = "tourvisor:calendar:last_run"
	tourvisorCalendarInterval   = 24 * time.Hour
	tourvisorCalendarCheckEvery = 30 * time.Minute
)

// StartTourvisorCalendarScheduler kalendar narxlarini kuniga bir marta yig'adi.
// TOURVISOR_CALENDAR=1 bo'lmaguncha ishlamaydi. Oxirgi ishga tushish vaqti
// Redis'da: konteyner qayta ishga tushsa ham pullik qidiruvlar qaytarilmaydi.
func StartTourvisorCalendarScheduler(ctx context.Context, db *sql.DB, cacheClient *cache.RedisCache) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("TOURVISOR_CALENDAR")), "1") {
		logger.Log.Info().Msg("tourvisor calendar scheduler disabled")
		return
	}
	interval := tourvisorCalendarRefreshInterval()
	logger.Log.Info().Dur("interval", interval).Msg("tourvisor calendar scheduler started")

	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Log.Info().Msg("tourvisor calendar scheduler stopped")
			return
		case <-timer.C:
		}
		// Belgi yozilmasa ishga tushmaymiz: aks holda har 30 daqiqada qayta yig'ib limitni yeb qo'yardi.
		if tourvisorCalendarDue(ctx, cacheClient, interval) && markTourvisorCalendarRun(ctx, cacheClient) {
			turvisorsearch.RefreshCalendars(ctx, db)
		}
		timer.Reset(tourvisorCalendarCheckEvery)
	}
}

func tourvisorCalendarRefreshInterval() time.Duration {
	hours, err := strconv.Atoi(strings.TrimSpace(os.Getenv("TOURVISOR_CALENDAR_INTERVAL_HOURS")))
	if err != nil || hours < 6 {
		return tourvisorCalendarInterval
	}
	return time.Duration(hours) * time.Hour
}

func tourvisorCalendarDue(ctx context.Context, cacheClient *cache.RedisCache, interval time.Duration) bool {
	raw, ok, err := cacheClient.GetRaw(ctx, tourvisorCalendarLastRunKey)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("tourvisor calendar last run unreadable — skip")
		return false
	}
	if !ok {
		return true
	}
	last, err := time.Parse(time.RFC3339, string(raw))
	if err != nil {
		return true
	}
	return time.Since(last) >= interval
}

// markTourvisorCalendarRun boshlanishdan oldin yoziladi: yarim yo'lda
// restart bo'lsa ham keyingi urinish faqat interval o'tgach bo'ladi.
func markTourvisorCalendarRun(ctx context.Context, cacheClient *cache.RedisCache) bool {
	stamp := []byte(time.Now().UTC().Format(time.RFC3339))
	if err := cacheClient.SetRaw(ctx, tourvisorCalendarLastRunKey, stamp, 7*24*time.Hour); err != nil {
		logger.Log.Warn().Err(err).Msg("tourvisor calendar last run not saved — skip")
		return false
	}
	return true
}
