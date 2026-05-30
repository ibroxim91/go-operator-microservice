package stream


import (
	"context"
	"go-operator-service/models"
	"go-operator-service/services"
	"sync"
	"github.com/joho/godotenv"
)

// init funksiyada .env faylni yuklab olamiz
func init() {
	_ = godotenv.Load(".env")
}

func StreamWorker(ctx context.Context, jobs <-chan models.Request, results chan<- models.Result, wg *sync.WaitGroup, hotelService *services.HotelService) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			if job.Istest {
				StreamHandleTestJob(ctx, job, results, hotelService) 
			} else {
				StreamHandleProdJob(ctx, job, results, hotelService)
			}
		}
	}
}

