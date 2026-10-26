package scheduler

import (
	"context"
	"time"

	"go-operator-service/logger"
	"go-operator-service/services"
)

func StartCurrencyRateScheduler(ctx context.Context, currency *services.CurrencyService) {
	logger.Log.Info().Msg("operator currency scheduler started")
	refreshOperatorRates(ctx, currency)
	for {
		wait := time.Until(nextCurrencyRefresh(time.Now()))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			logger.Log.Info().Msg("operator currency scheduler stopped")
			return
		case <-timer.C:
			refreshOperatorRates(ctx, currency)
		}
	}
}

func refreshOperatorRates(ctx context.Context, currency *services.CurrencyService) {
	if currency == nil {
		return
	}
	currency.RefreshOperatorRates(ctx)
}

func nextCurrencyRefresh(now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Tashkent")
	if err != nil {
		loc = time.FixedZone("UZT", 5*60*60)
	}
	local := now.In(loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), 6, 0, 0, 0, loc)
	if !local.Before(next) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
