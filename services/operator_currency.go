package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go-operator-service/logger"
)

const currencyFetchTimeout = 20 * time.Second

const defaultTourvisorRateOperator = "samo_tour"

type samoCurrencyRate struct {
	Value       string `json:"value"`
	CurrencyISO string `json:"currencyISO"`
}

type samoCurrency struct {
	Rates       []samoCurrencyRate `json:"rates"`
	CurrencyISO string             `json:"currencyISO"`
}

func TourvisorRateOperator() string {
	name := strings.TrimSpace(os.Getenv("TOURVISOR_RATE_OPERATOR"))
	if name == "" {
		return defaultTourvisorRateOperator
	}
	return name
}

func (s *CurrencyService) RateForOperator(ctx context.Context, operator string) float64 {
	operator = strings.TrimSpace(operator)
	if operator == "" || s == nil || s.cache == nil {
		return DefaultUsdRate
	}
	rate, found, err := s.cache.GetOperatorUsdRate(ctx, operator)
	if err != nil {
		logger.Log.Warn().Err(err).Str("operator", operator).Msg("operator usd rate cache read failed")
		return DefaultUsdRate
	}
	if !found || !UsableUsdRate(rate) {
		return DefaultUsdRate
	}
	return rate
}

func (s *SamoService) OperatorUsdRate(operator string) float64 {
	if s == nil || s.currencyService == nil {
		return DefaultUsdRate
	}
	return s.currencyService.RateForOperator(context.Background(), operator)
}

func (s *SamoService) TourvisorUsdRate() float64 {
	return s.OperatorUsdRate(TourvisorRateOperator())
}

func (s *CurrencyService) RefreshOperatorRates(ctx context.Context) {
	if s == nil || s.cache == nil {
		return
	}
	for _, cfg := range (&SamoService{}).getServiceConfigs() {
		if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.OAuthToken) == "" {
			continue
		}
		rate, err := fetchOperatorUsdRate(ctx, cfg.Name)
		if err != nil || !UsableUsdRate(rate) {
			logger.Log.Warn().
				Err(err).
				Str("operator", cfg.Name).
				Float64("rate", rate).
				Msg("operator usd rate refresh skipped")
			continue
		}
		if err := s.cache.SetOperatorUsdRate(ctx, cfg.Name, rate); err != nil {
			logger.Log.Warn().Err(err).Str("operator", cfg.Name).Msg("operator usd rate cache write failed")
			continue
		}
		logger.Log.Info().Str("operator", cfg.Name).Float64("uzs", rate).Msg("operator usd rate updated")
	}
}

func fetchOperatorUsdRate(ctx context.Context, operator string) (float64, error) {
	rawURL, err := currenciesURL(operator)
	if err != nil {
		return 0, err
	}
	signed, err := SignURL(operator, rawURL)
	if err != nil {
		return 0, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, currencyFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, signed, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("currency status " + strconv.Itoa(resp.StatusCode))
	}
	return uzsPerUsd(body)
}

func currenciesURL(operator string) (string, error) {
	cfg, ok := operatorConfig(operator)
	if !ok || strings.TrimSpace(cfg.BaseURL) == "" {
		return "", errors.New("no base url for operator " + operator)
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("samo_action", "api")
	q.Set("version", "1.0")
	q.Set("type", "json")
	q.Set("action", "SearchTour_CURRENCIES")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func uzsPerUsd(body []byte) (float64, error) {
	var wrapped struct {
		List json.RawMessage `json:"SearchTour_CURRENCIES"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return 0, err
	}
	if len(wrapped.List) == 0 {
		return 0, errors.New("usd to uzs rate not found")
	}
	var items []samoCurrency
	if wrapped.List[0] == '[' {
		if err := json.Unmarshal(wrapped.List, &items); err != nil {
			return 0, err
		}
	} else {
		var one samoCurrency
		if err := json.Unmarshal(wrapped.List, &one); err != nil {
			return 0, err
		}
		items = []samoCurrency{one}
	}
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.CurrencyISO), "USD") {
			continue
		}
		for _, rate := range item.Rates {
			if !strings.EqualFold(strings.TrimSpace(rate.CurrencyISO), "UZS") {
				continue
			}
			value, err := strconv.ParseFloat(strings.TrimSpace(rate.Value), 64)
			if err != nil {
				return 0, err
			}
			return value, nil
		}
	}
	return 0, errors.New("usd to uzs rate not found")
}
