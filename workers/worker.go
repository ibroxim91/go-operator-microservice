package workers

import (
    "bytes"
    "encoding/json"
    "fmt"
    "go-operator-service/models"
    "go-operator-service/utils"
    "io/ioutil"
    "net/http"
    "os"
    "sync"

    "github.com/joho/godotenv"
)

// init funksiyada .env faylni yuklab olamiz
func init() {
    _ = godotenv.Load(".env")
}

func worker(jobs <-chan models.Request, results chan<- models.Result, wg *sync.WaitGroup) {
    defer wg.Done()
    for job := range jobs {
        if job.Istest {
            handleTestJob(job, results)
        } else {
            handleProdJob(job, results)
        }
    }
}

// Test rejimdagi so‘rovlarni alohida funksiya
func handleTestJob(job models.Request, results chan<- models.Result) {
    payload := map[string]string{"url": job.Url}
    bodyBytes, _ := json.Marshal(payload)
    testURL := os.Getenv("TEST_URL")

   
    resp, err := http.Post(testURL, "application/json", bytes.NewBuffer(bodyBytes))
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

    formatPrices := []*models.Ticket{}
    for _, price := range parsed.SearchTour_PRICES.Prices {
        ticket := utils.TransformSamoPriceToTicket(
            price,
			job.Departure, job.Operator, job.DestCountryName, job.DestImageUrl,
			job.DestinationID,job.DepartureID, true,
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
func handleProdJob(job models.Request, results chan<- models.Result) {
    resp, err := http.Get(job.Url)
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

    formatPrices := []*models.Ticket{}
    for _, price := range parsed.SearchTour_PRICES.Prices {
        ticket := utils.TransformSamoPriceToTicket(
            price, job.Departure,
            job.Operator, job.DestCountryName, job.DestImageUrl, job.DestinationID,
            job.DepartureID, false,
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
