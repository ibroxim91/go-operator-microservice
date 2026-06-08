package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"go-operator-service/cache"
	"log"
	"strconv"
	"time"

	// "log"

	// "go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/stream"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
)

type StreamPayload struct {
	Prices     []*models.Ticket      `json:"prices"`
	Hotels     []models.HotelSummary `json:"hotels"`
	End        bool                  `json:"end"`
	Total      int                   `json:"total"`
	TotalPages int                   `json:"total_pages"`
	TotalItems int                   `json:"total_items"`
	FromCache  bool                  `json:"from_cache"`
}

func setStreamHeaders(c echo.Context) (http.ResponseWriter, http.Flusher, error) {
	rw := c.Response().Writer
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := rw.(http.Flusher)
	if !ok {
		return nil, nil, fmt.Errorf("streaming not supported")
	}
	return rw, flusher, nil
}

// yangi handler: async-samo/stream
func makeAsyncSamoTicketsStreamHandler(ctx context.Context, hotelService *services.HotelService, samoService *services.SamoService, cacheClient *cache.RedisCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		// tayyor parametrlar va jobs yaratish (sanoatdagi loadAsyncSamoTicketsResult bilan bir xil)
		samoParams, _, err := samoService.GetSamoParams(c)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		log.Println("samoParams stram ", samoParams)
		cacheKey := cache.BuildStreamCacheKey(samoParams)
		// response headerlarini sozlash
		rw, flusher, err := setStreamHeaders(c)
		cached, err := cacheClient.GetCachedStreamResult(
			ctx,
			cacheKey,
		)

		if err == nil && cached != nil {

			pageStr := samoParams["PRICEPAGE"]
			page, err := strconv.Atoi(pageStr)
			if err != nil {
				page = 1 // agar parse xato bo‘lsa default 1
			}
			if page <= 0 {
				page = 1
			}

			start := (page - 1) * 100
			end := start + 100
			log.Println("From Cache page = ", page, " Tickets len from cache ", len(cached.Tickets), " Page str ", pageStr)
			if start > len(cached.Tickets) {
				start = len(cached.Tickets)
			}

			if end > len(cached.Tickets) {
				end = len(cached.Tickets)
			}

			payload := StreamPayload{
				Prices:    cached.Tickets[start:end],
				Hotels:    cached.Hotels,
				End:       true,
				Total:     cached.Total,
                TotalPages:  (len(cached.Tickets) + 99) / 100,
				FromCache: true,
			}

			jsonData, _ := json.Marshal(payload)

			fmt.Fprintf(rw, "data: %s\n\n", jsonData)

			flusher.Flush()

			return nil
		}
		jobs, err := samoService.MakeURLs(samoParams)
		if err != nil || len(jobs) == 0 {
			return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(1))
		}

		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		// results channel va workerlarni ishga tushirish
		results := make(chan models.Result, 1000)
		go func() {
			// workerlarni ishga tushirish (workerCount = len(jobs) yoki max parallel)
			var wg sync.WaitGroup
			workerCount := min(len(jobs), 10) // misol uchun parallel 10
			jobsCh := jobsChanFromSlice(jobs)

			for i := 0; i < workerCount; i++ {
				wg.Add(1)
				go stream.StreamWorker(
					ctx,
					jobsCh,
					results,
					&wg,
					hotelService,
				)
			}
			wg.Wait()
			close(results)
		}()

		// results oqimini NDJSON formatida yuborish
		// enc := json.NewEncoder(rw)
		allData := make([]*models.Ticket, 0)
		stremaCount := 0
		hotelMap := map[string]models.HotelSummary{}
		// hotels := make([]models.HotelSummary, 0)

		for res := range results {
			if stremaCount < 100 {
				payload := StreamPayload{
					Prices: res.Prices,
				}
				jsonData, err := json.Marshal(payload)
				if err == nil {
					fmt.Fprintf(rw, "data: %s\n\n", jsonData)
					flusher.Flush()
				}
			}
			if len(res.Prices) > 0 {
				allData = append(allData, res.Prices...)
				stremaCount += len(res.Prices)
			}
			
			// Hotel summary yig'ish faqat oxirida ishlatiladi
			for _, ticket := range res.Prices {
				for _, hotel := range ticket.TicketHotel {
					key := fmt.Sprintf("%d|%s", hotel.ID, hotel.Name)
					if _, ok := hotelMap[key]; ok {
						continue
					}
					hotelMap[key] = models.HotelSummary{
						ID:          hotel.ID,
						Name:        hotel.Name,
						MealPlan:    hotel.MealPlan,
						Rating:      hotel.Rating,
						Operator:    ticket.Operator,
						Destination: ticket.Destination.Name,
					}
				}
			}
		}

		// Workerlar tugagach → yakuniy natija
		hotels := make([]models.HotelSummary, 0, len(hotelMap))
		for _, h := range hotelMap {
			hotels = append(hotels, h)
		}

		finalPayload := StreamPayload{
			Prices:     []*models.Ticket{},
			Hotels:     hotels,
			End:        true,
			Total:      stremaCount,
			TotalPages: (stremaCount + 99) / 100,
			TotalItems: stremaCount,
		}

		jsonData, err := json.Marshal(finalPayload)
		if err == nil {
			fmt.Fprintf(rw, "data: %s\n\n", jsonData)
			flusher.Flush()
		}

		cacheResult := &models.StreamCacheResult{
			Tickets: allData,
			Hotels:  hotels,
			Total:   len(allData),
		}

		go cacheClient.SetCachedStreamResult(
			context.Background(),
			cacheKey,
			cacheResult,
			10*time.Minute,
		)

		return nil
	}
}

// yordamchi: jobs channel yaratish
func jobsChanFromSlice(jobs []models.Request) <-chan models.Request {
	ch := make(chan models.Request, 200)
	go func() {
		for _, j := range jobs {
			ch <- j
		}
		close(ch)
	}()
	return ch
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
