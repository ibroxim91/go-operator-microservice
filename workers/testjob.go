package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/utils"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"
)

var client = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// Test rejimdagi so‘rovlarni alohida funksiya
func HandleTestJob(ctx context.Context, job models.Request, results chan<- models.Result, hotelService *services.HotelService) {
	payload := map[string]string{"url": job.Url}
	bodyBytes, _ := json.Marshal(payload)
	testURL := os.Getenv("TEST_URL")
	logger.Log.Info().
		Str("handler", "search-tours").
		Msg(fmt.Sprintf("Started Test Job For Operator: %s", job.Operator))
	logger.Log.Info().
		Str("handler", "search-tours").
		Msg(fmt.Sprintf("URL For Operator: %s", job.Url))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, testURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("error creating test request")
		results <- models.Result{Error: fmt.Sprintf("Error creating request: %v", err)}
		return
	}
	req.Header.Set("Content-Type", "application/json")
	//start := time.Now()
	resp, err := client.Do(req)

	//duration := time.Since(start)
	//logger.Log.Info().
	//	Str("operator", job.Operator).
	//	Dur("duration", duration).
	//	Msg("operator request finished")

	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("error fetching")
		results <- models.Result{Error: fmt.Sprintf("Error fetching: %s %v", job.Url, err)}
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("error reading body")
		results <- models.Result{Error: fmt.Sprintf("Error reading body: %v", err)}
		return
	}

	var parsed models.SearchTourResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("error parsing JSON")
		results <- models.Result{Error: fmt.Sprintf("Error parsing JSON: %v", err)}
		return
	}
	formatPrices := []*models.Ticket{}
	for _, price := range parsed.SearchTour_PRICES.Prices {
		if price.FreightExternal == "Y" {
			continue
		}
		ticket := utils.TransformSamoPriceToTicket(
			price, job.Departure,
			job.Operator, job.DestCountryName, job.DestImageUrl,
			job.CurrentUsdCourse, job.DestinationID,
			job.DepartureID, job.CountryID, hotelService, false, job.Url,
		)
		formatPrices = append(formatPrices, ticket)
	}

	//logger.Log.Info().
	//	Str("handler", "search-tours").
	//	Msg(fmt.Sprintf("len formatPrices for opertor %s: %d", job.Operator, len(formatPrices)))
	//logger.Log.Info().
	//	Str("handler", "search-tours").
	//	Msg(fmt.Sprintf("parsed.SearchTour_PRICES.Pager.Total for opertor %s: %d", job.Operator, parsed.SearchTour_PRICES.Pager.Total))
	// Agar 100 tadan kam bo‘lsa → boshqa pagelarni ham olish
	if !job.FirstPageOnly && len(formatPrices) < 500 && parsed.SearchTour_PRICES.Pager.Total > 1 {
		ch := make(chan []*models.Ticket)
		var wg sync.WaitGroup

		for page := 2; page <= parsed.SearchTour_PRICES.Pager.Total; page++ {
			 if page == 10{
                break
            }
			wg.Add(1)
			go func(p int) {
				defer wg.Done()
				FetchPage(ctx, p, job, hotelService, ch)
			}(page)
		}

		// Gorutinalarni yopish
		go func() {
			wg.Wait()
			close(ch)
		}()

		// Natijalarni yig‘ish
		for tickets := range ch {
			if tickets != nil {
				formatPrices = append(formatPrices, tickets...)
			}
		}
	}

	results <- models.Result{
		Prices:        formatPrices,
		Pager:         parsed.SearchTour_PRICES.Pager,
		Operator:      job.Operator,
		Departure:     job.Departure,
		DestCountry:   job.DestCountryName,
		DestinationID: job.DestinationID,
		DepartureID:   job.DepartureID,
	}
}