package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/utils"
	"net/http"
	"sync"
)

// Production rejimdagi so‘rovlar
func HandleProdJob(ctx context.Context, job models.Request, results chan<- models.Result, hotelService *services.HotelService) {
	signedURL, err := services.SignURL(job.Operator, job.Url)
	if err != nil {
		results <- models.Result{Error: fmt.Sprintf("Error creating request: %v", err)}
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
       logger.Log.Info().
        Str("handler", "search-tours").
        Str("url", job.Url).
        Msg("Starting production job")
	if err != nil {
		logger.Log.Error().
            Err(err).
            Str("handler", "search-tours").
            Msg("error creating request")
		results <- models.Result{Error: fmt.Sprintf("Error creating request: %v", err)}
		return
	}

	_, body, err := services.DoOperator(client, job.Operator, req)
	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("handler", "search-tours").
			Msg("error fetching")
		results <- models.Result{Error: fmt.Sprintf("Error fetching: %s %v", job.Url, err)}
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
        if price.FreightExternal == "Y"  {
            continue
        }
        if !models.IsBronBookable(price.Bron) {
            continue
        }
        if models.IsGDSTour(price.Tour) {
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

    // Operator bergan barcha sahifalar olinadi. Ilgari 9-sahifada to'xtardi
    // (1-sahifada 100+ narx bo'lsa — umuman olinmasdi): UAE'da takliflarning
    // ~80% i va mehmonxonalarning yarmi qidiruvga tushmasdi. Operatorga
    // yukni umumiy chegara (services.DoOperator) ushlab turadi.
    if !job.FirstPageOnly && parsed.SearchTour_PRICES.Pager.Total > 1 {
        ch := make(chan []*models.Ticket)
        var wg sync.WaitGroup

        lastPage := services.OperatorPagesToFetch(parsed.SearchTour_PRICES.Pager.Total)
        for page := 2; page <= lastPage; page++ {
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
