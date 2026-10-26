package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/stream"
	"go-operator-service/turvisorsearch"

	"github.com/labstack/echo/v4"
)

type StreamPayload struct {
	Prices           []*models.Ticket      `json:"prices"`
	Hotels           []models.HotelSummary `json:"hotels"`
	End              bool                  `json:"end"`
	Total            int                   `json:"total"`
	TotalPages       int                   `json:"total_pages"`
	TotalItems       int                   `json:"total_items"`
	FromCache        bool                  `json:"from_cache"`
	CurrentUsdCourse float64               `json:"current_usd_course"`
}

type streamUpdate struct {
	tickets []*models.Ticket
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

func envFlagEnabled(key string, defaultOn bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return defaultOn
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultOn
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+2)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func groupedStreamSnapshot(tickets []*models.Ticket, mode services.TicketSortMode) []*models.Ticket {
	cp := make([]*models.Ticket, 0, len(tickets))
	for _, ticket := range tickets {
		if ticket != nil {
			cp = append(cp, ticket)
		}
	}
	return services.ApplyStreamTicketSortMode(cp, mode)
}

func makeAsyncSamoTicketsStreamHandler(ctx context.Context, hotelService *services.HotelService, samoService *services.SamoService, cacheClient *cache.RedisCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		samoParams, _, userSpecifiedDate, err := samoService.GetSamoParams(c)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		log.Println("samoParams stram ", samoParams)
		currentUsdCourse := 0.0

		rw, flusher, err := setStreamHeaders(c)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		streamSAMO := envFlagEnabled("STREAM_SAMO", false)
		streamTourvisor := envFlagEnabled("STREAM_TOURVISOR", true)
		runTV := streamTourvisor && turvisorsearch.ShouldSearch(samoParams["OPERATOR"]) && samoService != nil && samoService.DB != nil
		sortMode := services.ParseTicketSortMode(samoParams)
		reqCtx := c.Request().Context()
		if reqCtx == nil {
			reqCtx = ctx
		}

		usePopularDestOnly := !runTV &&
			cache.IsPopularDestCacheEligible(c) &&
			cache.ShouldUsePopularDestCache(userSpecifiedDate, samoParams["CHECKIN_BEG"], samoParams["CHECKIN_END"])

		if usePopularDestOnly {
			popularCached, hit, popularErr := cacheClient.LookupPopularDestCache(ctx, cache.PopularDestinationsCacheKey)
			if popularErr != nil {
				logger.Log.Warn().
					Err(popularErr).
					Str("handler", "stream-samo/tickets").
					Msg("failed to load popular destination stream cache")
			} else if hit {
				filtered := services.FilterPopularDestAsyncResult(
					popularCached,
					c.QueryParam("departure"),
					c.QueryParam("destination"),
					c.QueryParam("country_id"),
					userSpecifiedDate,
					samoParams["CHECKIN_BEG"],
					samoParams["CHECKIN_END"],
				)
				if filtered != nil && len(filtered.Data.Results.Tickets) > 0 {
					page := parseRequestedPage(samoParams)
					payload := buildStreamPayloadFromAsyncCache(filtered, page)
					return writeStreamPayload(rw, flusher, payload)
				}
			}
		}

		cacheParams := cloneStringMap(samoParams)
		cacheParams["STREAM_SAMO"] = strconv.FormatBool(streamSAMO)
		cacheParams["STREAM_TOURVISOR"] = strconv.FormatBool(streamTourvisor)
		cacheKey := cache.BuildStreamCacheKey(cacheParams)

		if !usePopularDestOnly {
			cached, cacheErr := cacheClient.GetCachedStreamResult(ctx, cacheKey)
			if cacheErr == nil && cached != nil {
				page := parseRequestedPage(samoParams)
				payload := buildStreamPayloadFromCache(cached, page)
				payload.CurrentUsdCourse = currentUsdCourse
				return writeStreamPayload(rw, flusher, payload)
			}
		}

		var jobs []models.Request
		if streamSAMO {
			jobs, err = samoService.MakeURLs(samoParams)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
		}
		if !services.UsableUsdRate(currentUsdCourse) {
			currentUsdCourse = usdCourseForSearch(samoService, jobs, samoParams)
		}

		runSAMO := streamSAMO && len(jobs) > 0
		if !runSAMO && !runTV {
			return writeStreamPayload(rw, flusher, StreamPayload{
				Prices:           []*models.Ticket{},
				Hotels:           []models.HotelSummary{},
				End:              true,
				CurrentUsdCourse: currentUsdCourse,
			})
		}

		updates := make(chan streamUpdate, 32)
		var wg sync.WaitGroup

		if runSAMO {
			hotelService.BeginSearch()
			defer hotelService.EndSearch()

			wg.Add(1)
			go func() {
				defer wg.Done()
				results := make(chan models.Result, 1000)
				var workersWG sync.WaitGroup
				workerCount := min(len(jobs), 10)
				jobsCh := jobsChanFromSlice(jobs)
				for i := 0; i < workerCount; i++ {
					workersWG.Add(1)
					go stream.StreamWorker(reqCtx, jobsCh, results, &workersWG, hotelService)
				}
				go func() {
					workersWG.Wait()
					close(results)
				}()
				for res := range results {
					if len(res.Prices) == 0 {
						continue
					}
					select {
					case updates <- streamUpdate{tickets: res.Prices}:
					case <-reqCtx.Done():
						return
					}
				}
			}()
		}

		if runTV {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tvErr := turvisorsearch.Stream(reqCtx, samoService.DB, samoParams, samoService.TourvisorUsdRate(), func(batch []*models.Ticket) {
					if len(batch) == 0 {
						return
					}
					select {
					case updates <- streamUpdate{tickets: batch}:
					case <-reqCtx.Done():
					}
				})
				if tvErr != nil && tvErr != context.Canceled {
					logger.Log.Warn().
						Err(tvErr).
						Str("handler", "stream-samo/tickets").
						Msg("tourvisor stream failed")
				}
			}()
		}

		go func() {
			wg.Wait()
			close(updates)
		}()

		allRaw := make([]*models.Ticket, 0)
		flushSnapshot := func(end bool) error {
			grouped := groupedStreamSnapshot(allRaw, sortMode)
			hotels := services.BuildHotelSummaries(grouped)
			return writeStreamPayload(rw, flusher, StreamPayload{
				Prices:           grouped,
				Hotels:           hotels,
				End:              end,
				Total:            len(grouped),
				TotalPages:       (len(grouped) + 99) / 100,
				TotalItems:       len(grouped),
				CurrentUsdCourse: currentUsdCourse,
			})
		}

		for upd := range updates {
			if len(upd.tickets) > 0 {
				allRaw = append(allRaw, upd.tickets...)
				if err := flushSnapshot(false); err != nil {
					return err
				}
			}
		}

		if err := flushSnapshot(true); err != nil {
			return err
		}

		if !usePopularDestOnly {
			grouped := groupedStreamSnapshot(allRaw, sortMode)
			cacheResult := &models.StreamCacheResult{
				Tickets: grouped,
				Hotels:  services.BuildHotelSummaries(grouped),
				Total:   len(grouped),
			}
			go cacheClient.SetCachedStreamResult(
				context.Background(),
				cacheKey,
				cacheResult,
				10*time.Minute,
			)
		}

		return nil
	}
}

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
