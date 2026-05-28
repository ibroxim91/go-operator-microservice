package workers

import (
	"bytes"
	"context"
	"encoding/json"
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

func FetchPage(ctx context.Context, page int, job models.Request, hotelService *services.HotelService, ch chan<- []*models.Ticket) {

	testMode := os.Getenv("TEST") == "true"

	var resp *http.Response
	var err error
	var req *http.Request

	if testMode {
		// 🔹 Test rejimda POST qilish
		payload := map[string]string{"url": job.Url}
		bodyBytes, _ := json.Marshal(payload)
		testURL := os.Getenv("TEST_URL")

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, testURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("error creating test request")
			ch <- nil
			return
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		// 🔹 Production rejimda GET qilish
		url, err := buildURL(job.Url, page)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("error building URL")
			ch <- nil
			return
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			logger.Log.Error().
				Err(err).
				Str("handler", "search-tours").
				Msg("error creating prod request")
			ch <- nil
			return
		}
	}
	resp, err = http.DefaultClient.Do(req)

	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("request bind failed")
		ch <- nil
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
		ch <- nil
		return
	}

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

	ch <- tickets
}
