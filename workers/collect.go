package workers

import (
	"sync"
	"go-operator-service/models"
)

func CollectResults(jobsList []models.Request, workerCount int) models.ResultResponse {
    jobs := make(chan models.Request, len(jobsList))
    results := make(chan models.Result, len(jobsList))

    var wg sync.WaitGroup
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go worker(jobs, results, &wg)
    }

    for _, job := range jobsList {
        jobs <- job
    }
    close(jobs)

    go func() {
        wg.Wait()
        close(results)
    }()

    var allResults []*models.Ticket
	total := 0
    for res := range results {
		if len(res.Prices) == 100{
			total += res.Pager.Total *100
			
		}else if len(res.Prices) == 200{
			total += res.Pager.Total * 200
		}

        allResults = append(allResults, res.Prices...)
    }
    return models.ResultResponse{Prices: allResults, Total: total}
}