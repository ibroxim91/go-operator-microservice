package services

import (
	"context"

	"go-operator-service/cache"
)

const (
	DefaultUsdRate      = 12100.0
	minPlausibleUsdRate = 1000.0
)

type CurrencyService struct {
	cache *cache.RedisCache
}

func NewCurrencyService(cacheClient *cache.RedisCache) *CurrencyService {
	return &CurrencyService{cache: cacheClient}
}

func (s *CurrencyService) GetUsdRate(ctx context.Context) (float64, error) {
	if s == nil {
		return DefaultUsdRate, nil
	}
	return s.RateForOperator(ctx, TourvisorRateOperator()), nil
}

func UsableUsdRate(rate float64) bool {
	return rate >= minPlausibleUsdRate
}
