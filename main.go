package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-operator-service/cache"
	"go-operator-service/db"
	"go-operator-service/handlers"

	"go-operator-service/scheduler"
	"go-operator-service/services"

	"go-operator-service/logger"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()
	_ = godotenv.Load(".env")
	DBHost := os.Getenv("DBHost")
	DBPort := os.Getenv("DBPort")
	DBUser := os.Getenv("DBUser")
	DBPassword := os.Getenv("DBPassword")
	DBName := os.Getenv("DBName")
	logger.Init()
	
	conn, err := db.ConnectPostgres(DBHost, DBPort, DBUser, DBPassword, DBName)
	if err != nil {
		logger.Log.Fatal().
			Err(err).
			Msg("failed to connect redis")
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := services.PreloadHotelMappings(conn); err != nil {
		logger.Log.Warn().
			Err(err).
			Msg("failed to preload hotel mappings")
	}

	services.StartHotelMappingWorkers(ctx, conn, services.HotelMappingWorkerCount())

	hotelService := services.NewHotelService(conn)
	cacheClient, err := cache.NewRedisCache()
	if err != nil {
		logger.Log.Fatal().
			Err(err).
			Msg("failed to connect redis")
	}
	samoService := services.NewSamoService(conn, cacheClient)

	go scheduler.StartPopularDestinationsScheduler(ctx, conn, samoService, cacheClient, hotelService)

	frontendOrigin := os.Getenv("FRONTEND_ORIGIN")
	if frontendOrigin == "" {
		frontendOrigin = "http://localhost:3000"
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{frontendOrigin, "http://127.0.0.1:5500"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))
	e.Use(middleware.RequestID())
	handlers.RegisterRoutes(e, ctx, hotelService, samoService, cacheClient)

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)

			logger.Log.Info().
				Str("request_id", c.Response().Header().Get(echo.HeaderXRequestID)).
				Str("method", c.Request().Method).
				Str("path", c.Request().URL.Path).
				Int("status", c.Response().Status).
				Dur("latency", time.Since(start)).
				Str("ip", c.RealIP()).
				Msg("http request")

			return err
		}
	})

	go func() {
		logger.Log.Info().
			Msg("pprof started on :6060")

		if err := http.ListenAndServe(
			":6060",
			nil,
		); err != nil {
			logger.Log.Error().
				Err(err).
				Msg("pprof server error")
		}
	}()

	srv := &http.Server{Addr: ":8088", Handler: e}
	go func() {
		if err := e.StartServer(srv); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server ", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	// Cancel server context → workerlar ham to‘xtaydi
	cancel()
	services.WaitHotelMappingWorkers(5 * time.Second)

	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelTimeout()
	if err := e.Shutdown(ctxTimeout); err != nil {
		e.Logger.Fatal(err)
	}
}
