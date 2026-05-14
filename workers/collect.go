package workers

import (
	"context"
	"go-operator-service/models"
	"go-operator-service/services"
	"log"
	"sync"
)

func CollectResults(ctx context.Context, jobsList []models.Request, workerCount int, hotelService *services.HotelService) models.ResultResponse {
	jobs := make(chan models.Request, len(jobsList))
	results := make(chan models.Result, len(jobsList))

	log.Println("jobsList len ", len(jobsList))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(ctx, jobs, results, &wg, hotelService)
	}

	for _, job := range jobsList {
		select {
		case <-ctx.Done():
			break
		case jobs <- job:
		}
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var allResults []*models.Ticket
	total := 0
	page := 1
	if len(jobsList) > 0 {
		page = jobsList[0].Page
	}
	for res := range results {
		total += res.Pager.Total * len(res.Prices)
		allResults = append(allResults, res.Prices...)
	}
	log.Println("REsults len ", len(allResults))
	return models.ResultResponse{Prices: allResults, Total: total, Page: page}
}
