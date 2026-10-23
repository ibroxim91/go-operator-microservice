package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
)

const (
	CBURL               = "https://cbu.uz/uz/arkhiv-kursov-valyut/json/"
	DefaultUsdRate      = 12100.0
	usdRateCacheTTL     = 14 * time.Hour
	fallbackUsdRateTTL  = 15 * time.Minute
	cbuTimeout          = 5 * time.Second
	minPlausibleUsdRate = 1000.0
)

type CurrencyService struct {
	cache *cache.RedisCache
}

type currencyRate struct {
	Ccy  string `json:"Ccy"`
	Rate string `json:"Rate"`
}

func NewCurrencyService(cacheClient *cache.RedisCache) *CurrencyService {
	return &CurrencyService{cache: cacheClient}
}

func (s *CurrencyService) GetUsdRate(ctx context.Context) (float64, error) {
	if s.cache != nil {
		cachedRate, found, err := s.cache.GetUsdRate(ctx)
		if err == nil && found && cachedRate >= minPlausibleUsdRate {
			return cachedRate, nil
		}
	}

	rate, err := fetchCbuUsdRate(ctx)
	if err != nil || rate < minPlausibleUsdRate {
		logger.Log.Warn().Err(err).Float64("rate", rate).Msg("cbu usd rate unavailable, using default 12100")
		if s.cache != nil {
			_ = s.cache.SetUsdRate(ctx, DefaultUsdRate, fallbackUsdRateTTL)
		}
		return DefaultUsdRate, nil
	}

	if s.cache != nil {
		_ = s.cache.SetUsdRate(ctx, rate, usdRateCacheTTL)
	}
	return rate, nil
}

func UsableUsdRate(rate float64) bool {
	return rate >= minPlausibleUsdRate
}

func fetchCbuUsdRate(ctx context.Context) (float64, error) {
	reqCtx, cancel := context.WithTimeout(ctx, cbuTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, CBURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, errors.New("API so'rovi xatosi: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("API so'rovi xatosi: status code " + strconv.Itoa(resp.StatusCode))
	}

	var rates []currencyRate
	if err := json.NewDecoder(resp.Body).Decode(&rates); err != nil {
		return 0, errors.New("JSON dekoding xatosi: " + err.Error())
	}

	for _, item := range rates {
		if item.Ccy != "USD" {
			continue
		}
		rate, err := strconv.ParseFloat(item.Rate, 64)
		if err != nil {
			return 0, errors.New("USD kursini pars qilish xatosi: " + err.Error())
		}
		return rate, nil
	}

	return 0, errors.New("USD kursi topilmadi")
}
