package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/utils"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
)

// Test rejimdagi so‘rovlarni alohida funksiya
func HandleTestJob(ctx context.Context, job models.Request, results chan<- models.Result, hotelService *services.HotelService) {
	payload := map[string]string{"url": job.Url}
	bodyBytes, _ := json.Marshal(payload)
	testURL := os.Getenv("TEST_URL")
	log.Println("Started Test Job For Operator: ", job.Operator)
	log.Println("URL For Operator: ", job.Url)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, testURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Fatal("requst error  testURL", err)
		results <- models.Result{Error: fmt.Sprintf("Error creating request: %v", err)}
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal("N request ", err)
		results <- models.Result{Error: fmt.Sprintf("Error fetching: %s %v", job.Url, err)}
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error red body", err)
		results <- models.Result{Error: fmt.Sprintf("Error reading body: %v", err)}
		return
	}

	var parsed models.SearchTourResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Fatal("Errrr unmarshal json", err)
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
			job.DepartureID, job.CountryID, hotelService, false,
		)
		formatPrices = append(formatPrices, ticket)
	}

	fmt.Println("len formatPrices for opertor ", job.Operator, len(formatPrices))
	fmt.Println("parsed.SearchTour_PRICES.Pager.Total for opertor", job.Operator, parsed.SearchTour_PRICES.Pager.Total)
	// Agar 100 tadan kam bo‘lsa → boshqa pagelarni ham olish
	if len(formatPrices) < 500 && parsed.SearchTour_PRICES.Pager.Total > 1 {
		ch := make(chan []*models.Ticket)
		var wg sync.WaitGroup

		for page := 2; page <= parsed.SearchTour_PRICES.Pager.Total; page++ {
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
