package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/utils"
	"io/ioutil"
	"net/http"
	"sync"

	"go-operator-service/logger"
)

/// Production rejimdagi so‘rovlar
func StreamHandleProdJob(
    ctx context.Context,
    job models.Request,
    results chan<- models.Result,
    hotelService *services.HotelService,
) {
    logger.Log.Info().
		Str("handler", "search-tours").
		Str("url", job.Url).
		Msg("Starting production job")
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.Url, nil)
    if err != nil {
        results <- models.Result{Error: fmt.Sprintf("Error creating request: %v", err)}
        return
    }

    resp, err := client.Do(req)
    if err != nil {
        results <- models.Result{Error: fmt.Sprintf("Error fetching: %s %v", job.Url, err)}
        return
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        results <- models.Result{Error: fmt.Sprintf("Error reading body: %v", err)}
        return
    }

    var parsed models.SearchTourResponse
    if err := json.Unmarshal(body, &parsed); err != nil {
        results <- models.Result{Error: fmt.Sprintf("Error parsing JSON: %v", err)}
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
            job.Url,
        )
        page1Tickets = append(page1Tickets, ticket)
    }

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
        wg.Add(1)
        go func(p int) {
            defer wg.Done()
            StreamFetchPage(ctx, p, job, hotelService, results)
        }(page)
    }
    wg.Wait()
}
