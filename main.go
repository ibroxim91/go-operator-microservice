package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go-operator-service/cache"
	"go-operator-service/db"
	"go-operator-service/handlers"

	"go-operator-service/scheduler"
	"go-operator-service/services"
	"go-operator-service/turvisorsearch"

	"go-operator-service/logger"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// defaultFrontOrigins — prod + local; FRONTEND_ORIGIN (vergul bilan) qo'shimcha originlar.
var defaultFrontOrigins = []string{
	"https://simpletravel.uz",
	"http://simpletravel.uz",
	"https://www.simpletravel.uz",
	"https://simple-travel.felixits.uz",
	"http://localhost:3000",
	"http://127.0.0.1:3000",
	"http://localhost:3001",
	"http://127.0.0.1:5500",
}

func corsAllowOrigins() []string {
	seen := make(map[string]struct{}, len(defaultFrontOrigins)+4)
	out := make([]string, 0, len(defaultFrontOrigins)+4)
	add := func(raw string) {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			return
		}
		if _, ok := seen[origin]; ok {
			return
		}
		seen[origin] = struct{}{}
		out = append(out, origin)
	}
	for _, origin := range defaultFrontOrigins {
		add(origin)
	}
	for _, part := range strings.Split(os.Getenv("FRONTEND_ORIGIN"), ",") {
		add(part)
	}
	return out
}

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

	if err := services.PreloadCountryVisaMap(conn); err != nil {
		logger.Log.Warn().
			Err(err).
			Msg("failed to preload country visa map")
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
	turvisorsearch.Configure(cacheClient)

	go scheduler.StartPopularDestinationsScheduler(ctx, conn, samoService, cacheClient, hotelService)
	go scheduler.StartHomeOffersScheduler(ctx, conn, samoService, cacheClient, hotelService)
	go scheduler.StartHotToursScheduler(ctx, cacheClient)
	go scheduler.StartTourvisorCalendarScheduler(ctx, conn, cacheClient)
	go scheduler.StartCurrencyRateScheduler(ctx, samoService.Currency())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: corsAllowOrigins(),
		AllowMethods: []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"Accept-Language",
			"X-Requested-With",
		},
		ExposeHeaders: []string{echo.HeaderXRequestID},
		MaxAge:        86400,
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
