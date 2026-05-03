package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/labstack/echo/v4"
    "go-operator-service/models"
    "go-operator-service/workers"
)

func main() {
    e := echo.New()

    // Server context
    ctx, cancel := context.WithCancel(context.Background())

    e.POST("/search-tours", func(c echo.Context) error {
        var jobs []models.Request
        if err := c.Bind(&jobs); err != nil {
            return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
        }

        results := workers.CollectResults(ctx, jobs, len(jobs))
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
