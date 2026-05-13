package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-operator-service/db"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/workers"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
)

func main() {
	e := echo.New()
	 _ = godotenv.Load(".env")
    DBHost := os.Getenv("DBHost")
    DBPort := os.Getenv("DBPort")
    DBUser := os.Getenv("DBUser")
    DBPassword := os.Getenv("DBPassword")
    DBName := os.Getenv("DBName")

	conn, err := db.ConnectPostgres(DBHost, DBPort, DBUser, DBPassword, DBName, )
    if err != nil {
        log.Fatalf("Failed to connect to Postgres: %v", err)
    }
    defer conn.Close()
	hotelService := services.NewHotelService(conn)
	// Server context
	ctx, cancel := context.WithCancel(context.Background())

	e.POST("/search-tours", func(c echo.Context) error {
		var jobs []models.Request
		if err := c.Bind(&jobs); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		results := workers.CollectResults(ctx, jobs, len(jobs), hotelService)

		return c.JSON(http.StatusOK, results)
	})

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

	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelTimeout()
	if err := e.Shutdown(ctxTimeout); err != nil {
		e.Logger.Fatal(err)
	}
}
