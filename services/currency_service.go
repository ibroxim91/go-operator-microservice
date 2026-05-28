package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const CBURL = "https://cbu.uz/uz/arkhiv-kursov-valyut/json/"

type CurrencyService struct{}

type currencyRate struct {
	Ccy  string `json:"Ccy"`
	Rate string `json:"Rate"`
}

var (
	currencyCacheMu     sync.Mutex
	currencyCacheRate   float64
	currencyCacheExpiry time.Time
)

func (CurrencyService) GetUsdRate() (float64, error) {
	currencyCacheMu.Lock()
	if time.Now().Before(currencyCacheExpiry) && currencyCacheRate > 0 {
		rate := currencyCacheRate
		currencyCacheMu.Unlock()
		return rate, nil
	}
	currencyCacheMu.Unlock()

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
		if item.Ccy == "USD" {
			rate, err := strconv.ParseFloat(item.Rate, 64)
			if err != nil {
				return 0, errors.New("USD kursini pars qilish xatosi: " + err.Error())
			}

			currencyCacheMu.Lock()
			currencyCacheRate = rate
			currencyCacheExpiry = time.Now().Add(24 * time.Hour)
			currencyCacheMu.Unlock()

			return rate, nil
		}
	}

	return 0, errors.New("USD kursi topilmadi")
}
