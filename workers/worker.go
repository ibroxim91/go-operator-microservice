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

	"github.com/joho/godotenv"
)

// init funksiyada .env faylni yuklab olamiz
func init() {
	_ = godotenv.Load(".env")
}

func worker(ctx context.Context, jobs <-chan models.Request, results chan<- models.Result, wg *sync.WaitGroup, hotelService *services.HotelService) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			// Agar job ishlayapti bo‘lsa, uni tugatib keyin return qilamiz
			// Yangi job olmaymiz
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			if job.Istest {
				handleTestJob(ctx, job, results, hotelService) // ctx beramiz
			} else {
				handleProdJob(ctx, job, results, hotelService)
			}
		}
	}
}

// Test rejimdagi so‘rovlarni alohida funksiya
func handleTestJob(ctx context.Context, job models.Request, results chan<- models.Result, hotelService *services.HotelService) {
	payload := map[string]string{"url": job.Url}
	bodyBytes, _ := json.Marshal(payload)
	testURL := os.Getenv("TEST_URL")
	log.Println("Started Test Job For Operator: ", job.Operator)
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
		// if price.Bron == false {
		// 	continue
		// }
		ticket := utils.TransformSamoPriceToTicket(
			price,
			job.Departure, job.Operator, job.DestCountryName, job.DestImageUrl,
			job.CurrentUsdCourse,
			job.DestinationID, job.DepartureID, job.CountryID, hotelService, true,
		)
		formatPrices = append(formatPrices, ticket)
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

// Production rejimdagi so‘rovlar
func handleProdJob(ctx context.Context, job models.Request, results chan<- models.Result, hotelService *services.HotelService) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.Url, nil)
	log.Println("Started PROD Job For Operator: ", job.Operator)
	if err != nil {
		log.Fatal("Errrr NewRequestWithContext", err)
		results <- models.Result{Error: fmt.Sprintf("Error creating request: %v", err)}
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		results <- models.Result{Error: fmt.Sprintf("Error fetching: %s %v", job.Url, err)}
		log.Fatal("Errrr DefaultClient", err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Errrr ReadAll", err)
		results <- models.Result{Error: fmt.Sprintf("Error reading body: %v", err)}
		return
	}

	var parsed models.SearchTourResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Fatal("Errrr Unmarshal", err)
		results <- models.Result{Error: fmt.Sprintf("Error parsing JSON: %v", err)}
		return
	}

	formatPrices := []*models.Ticket{}
	for _, price := range parsed.SearchTour_PRICES.Prices {
		if price.FreightExternal == "Y" {
			continue
		}
		// if price.Bron == false {
		// 	continue
		// }
		ticket := utils.TransformSamoPriceToTicket(
			price, job.Departure,
			job.Operator, job.DestCountryName, job.DestImageUrl,
			job.CurrentUsdCourse, job.DestinationID,
			job.DepartureID, job.CountryID, hotelService, false,
		)
		formatPrices = append(formatPrices, ticket)
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
