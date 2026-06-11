package services

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"go-operator-service/cache"
	"go-operator-service/logger"
	"go-operator-service/repository"
)

const (
	defaultHotelMappingWorkers = 5
	hotelMappingQueueSize      = 10000
)

// HotelMappingJob represents a single hotel mapping persistence task.
type HotelMappingJob struct {
	Operator        string
	OperatorHotelID int
	HotelID         int
}

var (
	hotelMappingQueue     chan HotelMappingJob
	hotelMappingWorkersWg sync.WaitGroup
	hotelMappingPending   sync.Map
	hotelMappingInitOnce  sync.Once
)

// HotelMappingWorkerCount returns the configured worker count (env: HOTEL_MAPPING_WORKERS).
func HotelMappingWorkerCount() int {
	raw := os.Getenv("HOTEL_MAPPING_WORKERS")
	if raw == "" {
		return defaultHotelMappingWorkers
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultHotelMappingWorkers
	}
	return n
}

// StartHotelMappingWorkers starts a pool of background workers at application startup.
func StartHotelMappingWorkers(ctx context.Context, db *sql.DB, workerCount int) {
	hotelMappingInitOnce.Do(func() {
		if workerCount < 1 {
			workerCount = defaultHotelMappingWorkers
		}

		hotelMappingQueue = make(chan HotelMappingJob, hotelMappingQueueSize)

		for i := 0; i < workerCount; i++ {
			hotelMappingWorkersWg.Add(1)
			go func() {
				defer hotelMappingWorkersWg.Done()
				HotelMappingWorker(ctx, db, hotelMappingQueue)
			}()
		}

		logger.Log.Info().
			Int("workers", workerCount).
			Int("queue_size", hotelMappingQueueSize).
			Msg("hotel mapping workers started")
	})
}

// HotelMappingWorker processes mapping jobs until the context is cancelled.
func HotelMappingWorker(ctx context.Context, db *sql.DB, jobs <-chan HotelMappingJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			processHotelMappingJob(ctx, db, job)
		}
	}
}

func processHotelMappingJob(ctx context.Context, db *sql.DB, job HotelMappingJob) {
	key := cache.HotelMappingKey(job.Operator, job.OperatorHotelID)
	defer hotelMappingPending.Delete(key)

	if err := SaveHotelMapping(ctx, db, job); err != nil {
		logger.Log.Warn().
			Err(err).
			Str("operator", job.Operator).
			Int("operator_hotel_id", job.OperatorHotelID).
			Int("hotel_id", job.HotelID).
			Msg("failed to save hotel mapping")
	}
}

// EnqueueHotelMapping schedules a mapping write if it is not already cached or pending.
func EnqueueHotelMapping(job HotelMappingJob) {
	if _, ok := cache.GetMappedHotelID(job.Operator, job.OperatorHotelID); ok {
		return
	}

	key := cache.HotelMappingKey(job.Operator, job.OperatorHotelID)
	if _, loaded := hotelMappingPending.LoadOrStore(key, struct{}{}); loaded {
		return
	}

	if hotelMappingQueue == nil {
		hotelMappingPending.Delete(key)
		logger.Log.Warn().Msg("hotel mapping queue is not initialized")
		return
	}

	select {
	case hotelMappingQueue <- job:
	default:
		hotelMappingPending.Delete(key)
		logger.Log.Warn().
			Str("operator", job.Operator).
			Int("operator_hotel_id", job.OperatorHotelID).
			Msg("hotel mapping queue is full, job dropped")
	}
}

// SaveHotelMapping persists a mapping when it is not present in cache.
func SaveHotelMapping(ctx context.Context, db *sql.DB, job HotelMappingJob) error {
	if _, ok := cache.GetMappedHotelID(job.Operator, job.OperatorHotelID); ok {
		return nil
	}

	if err := repository.SaveHotelMapping(
		ctx,
		db,
		job.Operator,
		job.OperatorHotelID,
		job.HotelID,
	); err != nil {
		return fmt.Errorf("save hotel mapping: %w", err)
	}

	cache.SetHotelMapping(job.Operator, job.OperatorHotelID, job.HotelID)
	return nil
}

// WaitHotelMappingWorkers blocks until all workers exit or the timeout elapses.
func WaitHotelMappingWorkers(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		hotelMappingWorkersWg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		logger.Log.Warn().
			Dur("timeout", timeout).
			Msg("hotel mapping workers shutdown timed out")
	}
}
