package stream

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
	"os"
	"strconv"

	"net/http"
	"net/url"
)

func buildURL(base string, page int) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("PRICEPAGE", strconv.Itoa(page)) // mavjud bo‘lsa almashtiradi
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func StreamFetchPage(
    ctx context.Context,
    page int,
    job models.Request,
    hotelService *services.HotelService,
    results chan<- models.Result,
){

	testMode := os.Getenv("TEST") == "true"

	var resp *http.Response
	var err error
	var req *http.Request
	logger.Log.Info().
		Int("page", page).
		Str("url", job.Url).
		Msg("FETCH PAGE")

	url, err := buildURL(job.Url, page)
		logger.Log.Info().
			Str("handler", "search-tours").
			Str("url", url).
			Msg("Starting production job")
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("error building URL")
			results <- models.Result{
				Error: err.Error(),
			}
			return
		}
	if testMode {
		// 🔹 Test rejimda POST qilish
		payload := map[string]string{"url": url}
		bodyBytes, _ := json.Marshal(payload)
		testURL := os.Getenv("TEST_URL")
		logger.Log.Info().
			Str("handler", "search-tours").
			Str("url", url).
			Msg("Starting test job")
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, testURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("error creating test request")
			results <- models.Result{
				Error: err.Error(),
			}
			return
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		// 🔹 Production rejimda GET qilish
		
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("error creating prod request")
			results <- models.Result{
				Error: err.Error(),
			}
			return
		}
	}
	resp, err = client.Do(req)

	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("request bind failed")
		results <- models.Result{
			Error: err.Error(),
		}
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	var parsed models.SearchTourResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("error parsing page")
		results <- models.Result{
			Error: err.Error(),
		}
		return
	}
	logger.Log.Info().
		Str("handler", "search-tours").
		Msg(fmt.Sprintf("len prices for opertor %s: %d page %d", job.Operator, len(parsed.SearchTour_PRICES.Prices), page))
	tickets := []*models.Ticket{}

	for _, price := range parsed.SearchTour_PRICES.Prices {
		if price.FreightExternal == "Y" {
			continue
		}
		ticket := utils.TransformSamoPriceToTicket(
			price, job.Departure,
			job.Operator, job.DestCountryName, job.DestImageUrl,
			job.CurrentUsdCourse, job.DestinationID,
			job.DepartureID, job.CountryID, hotelService, false,
		)
		tickets = append(tickets, ticket)
	}
	if len(tickets) > 0 {
		results <- models.Result{
			Prices:        tickets,
			Operator:      job.Operator,
			Departure:     job.Departure,
			DestCountry:   job.DestCountryName,
			DestinationID: job.DestinationID,
			DepartureID:   job.DepartureID,
			Page:          page,
		}
	}
}
