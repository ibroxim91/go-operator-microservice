package handlers

import (
	"context"
	"encoding/json"
	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/workers"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// yangi handler: async-samo/stream
func makeAsyncSamoTicketsStreamHandler(ctx context.Context, hotelService *services.HotelService, samoService *services.SamoService, cacheClient *cache.RedisCache) echo.HandlerFunc {
    return func(c echo.Context) error {
        // tayyor parametrlar va jobs yaratish (sanoatdagi loadAsyncSamoTicketsResult bilan bir xil)
        samoParams, _, err := samoService.GetSamoParams(c)
        if err != nil {
            return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
        }
        jobs, err := samoService.MakeURLs(samoParams)
        if err != nil || len(jobs) == 0 {
            return c.JSON(http.StatusOK, buildEmptyAsyncSamoResult(1))
        }

        // response headerlarini sozlash
        rw := c.Response().Writer
        c.Response().Header().Set(echo.HeaderContentType, "application/x-ndjson")
        c.Response().Header().Set("Transfer-Encoding", "chunked")
        c.Response().WriteHeader(http.StatusOK)

        flusher, ok := rw.(http.Flusher)
        if !ok {
            return c.JSON(http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
        }

        // results channel va workerlarni ishga tushirish
        results := make(chan models.Result, len(jobs))
        go func() {
            // workerlarni ishga tushirish (workerCount = len(jobs) yoki max parallel)
            var wg sync.WaitGroup
            workerCount := min(len(jobs), 10) // misol uchun parallel 10
            for i := 0; i < workerCount; i++ {
                wg.Add(1)
                go workers.Worker(ctx, jobsChanFromSlice(jobs), results, &wg, hotelService)
            }
            wg.Wait()
            close(results)
        }()

        // results oqimini NDJSON formatida yuborish
        enc := json.NewEncoder(rw)
        for res := range results {
            // har bir res ni NDJSON qatoriga aylantirish
            if err := enc.Encode(res); err != nil {
                // yozishda xato bo'lsa, log va break
                logger.Log.Error().Err(err).Msg("failed to encode stream result")
                break
            }
            flusher.Flush()
            // optional: kichik kechikish yoki rate limit
            time.Sleep(5 * time.Millisecond)
        }

        return nil
    }
}

// yordamchi: jobs channel yaratish
func jobsChanFromSlice(jobs []models.Request) <-chan models.Request {
    ch := make(chan models.Request, len(jobs))
    go func() {
        for _, j := range jobs {
            ch <- j
        }
        close(ch)
    }()
    return ch
}

func min(a, b int) int {
    if a < b { return a }
    return b
}
