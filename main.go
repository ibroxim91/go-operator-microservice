package main

import (
    "net/http"

    "github.com/labstack/echo/v4"
    "go-operator-service/models"
    "go-operator-service/workers"
)

func main() {
    e := echo.New()

    // POST /search-tours
    e.POST("/search-tours", func(c echo.Context) error {
        var jobs []models.Request
        if err := c.Bind(&jobs); err != nil {
            return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
        }

        // Worker pool orqali natijalarni yig‘ish
        results := workers.CollectResults(jobs, len(jobs))

        return c.JSON(http.StatusOK, results)
    })

    e.Logger.Fatal(e.Start(":8088"))
}
