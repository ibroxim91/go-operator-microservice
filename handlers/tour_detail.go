package handlers

import (
	"net/http"
	"strings"

	"go-operator-service/cache"
	"go-operator-service/hottours"
	"go-operator-service/logger"
	"go-operator-service/services"

	"github.com/labstack/echo/v4"
)

func makeTourvisorTourHandler(cacheClient *cache.RedisCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		tourID := strings.TrimSpace(c.Param("tourId"))
		course, _ := services.NewCurrencyService(cacheClient).GetUsdRate(c.Request().Context())
		ticket, err := hottours.FetchTour(c.Request().Context(), tourID, course)
		if err != nil {
			logger.Log.Error().Err(err).Str("handler", "async-samo/tours").Str("tour_id", tourID).Msg("tour detail failed")
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "tour detail unavailable"})
		}
		return c.JSON(http.StatusOK, ticket)
	}
}
