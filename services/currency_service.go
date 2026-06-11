package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go-operator-service/cache"
)

const (
	CBURL             = "https://cbu.uz/uz/arkhiv-kursov-valyut/json/"
	usdRateCacheTTL   = 14 * time.Hour
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
		if err == nil && found && cachedRate > 0 {
			return cachedRate, nil
		}
	}

	resp, err := http.Get(CBURL)
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

		if s.cache != nil {
			_ = s.cache.SetUsdRate(ctx, rate, usdRateCacheTTL)
		}

		return rate, nil
	}

	return 0, errors.New("USD kursi topilmadi")
}
