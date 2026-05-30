package stream

import (
	"bytes"
	"context"
	"encoding/json"
	
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
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// Test rejimdagi so‘rovlarni alohida funksiya
func StreamHandleTestJob(
    ctx context.Context,
    job models.Request,
    results chan<- models.Result,
    hotelService *services.HotelService,
) {

    payload := map[string]string{
        "url": job.Url,
    }

    bodyBytes, _ := json.Marshal(payload)

    testURL := os.Getenv("TEST_URL")

    req, err := http.NewRequestWithContext(
        ctx,
        http.MethodPost,
        testURL,
        bytes.NewBuffer(bodyBytes),
    )

    if err != nil {
        results <- models.Result{
            Error: err.Error(),
        }
        return
    }

    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)

    if err != nil {
        results <- models.Result{
            Error: err.Error(),
        }
        return
    }

    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if err != nil {
        results <- models.Result{
            Error: err.Error(),
        }
        return
    }

    var parsed models.SearchTourResponse

    if err := json.Unmarshal(body, &parsed); err != nil {
        results <- models.Result{
            Error: err.Error(),
        }
        return
    }

    // =========================
    // PAGE 1 IMMEDIATE STREAM
    // =========================

    page1Tickets := []*models.Ticket{}

    for _, price := range parsed.SearchTour_PRICES.Prices {

        if price.FreightExternal == "Y" {
            continue
        }

        ticket := utils.TransformSamoPriceToTicket(
            price,
            job.Departure,
            job.Operator,
            job.DestCountryName,
            job.DestImageUrl,
            job.CurrentUsdCourse,
            job.DestinationID,
            job.DepartureID,
            job.CountryID,
            hotelService,
            false,
        )

        page1Tickets = append(page1Tickets, ticket)
    }

    // DARHOL USERGA YUBOR
	if len(page1Tickets) > 0 {
		results <- models.Result{
			Prices:        page1Tickets,
			Operator:      job.Operator,
			Departure:     job.Departure,
			DestCountry:   job.DestCountryName,
			DestinationID: job.DestinationID,
			DepartureID:   job.DepartureID,
			Page:          1,
		}
    }

    // =========================
    // OTHER PAGES PARALLEL
    // =========================

    totalPages := parsed.SearchTour_PRICES.Pager.Total

    if totalPages <= 1 {
        return
    }

    var wg sync.WaitGroup

    for page := 2; page <= totalPages; page++ {

        if page > 10 {
            break
        }

        wg.Add(1)

        go func(p int) {

            defer wg.Done()

            StreamFetchPage(
                ctx,
                p,
                job,
                hotelService,
                results,
            )

        }(page)
    }

    wg.Wait()
}