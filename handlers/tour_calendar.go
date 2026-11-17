package handlers

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"go-operator-service/services"
	"go-operator-service/turvisorsearch"
	"go-operator-service/utils"

	"github.com/labstack/echo/v4"
)

type tourCalendarDay struct {
	PriceUSD     float64 `json:"price_usd"`
	PriceUZS     int     `json:"price_uzs"`
	PerPersonUZS int     `json:"per_person_uzs"`
	PerPersonUSD float64 `json:"per_person_usd"`
}

type tourCalendarResponse struct {
	Ready      bool                       `json:"ready"`
	Adults     int                        `json:"adults"`
	NightsFrom int                        `json:"nights_from"`
	NightsTo   int                        `json:"nights_to"`
	UpdatedAt  string                     `json:"updated_at,omitempty"`
	Days       map[string]tourCalendarDay `json:"days"`
}

// makeTourCalendarHandler — kalendar narxlari, faqat Redis'dan o'qiladi.
// GET /async-samo/tour-calendar?departure=21&country_id=25
func makeTourCalendarHandler(samoService *services.SamoService) echo.HandlerFunc {
	return func(c echo.Context) error {
		empty := tourCalendarResponse{Days: map[string]tourCalendarDay{}}
		departure, errDep := strconv.Atoi(strings.TrimSpace(c.QueryParam("departure")))
		country, errCountry := strconv.Atoi(strings.TrimSpace(c.QueryParam("country_id")))
		if errDep != nil || errCountry != nil || departure <= 0 || country <= 0 {
			return c.JSON(http.StatusOK, empty)
		}
		cal, ok, err := turvisorsearch.LoadCalendar(c.Request().Context(), departure, country)
		if err != nil || !ok || cal == nil {
			return c.JSON(http.StatusOK, empty)
		}

		rate := services.DefaultUsdRate
		if samoService != nil {
			rate = samoService.TourvisorUsdRate()
		}
		adults := cal.Adults
		if adults <= 0 {
			adults = 2
		}
		days := make(map[string]tourCalendarDay, len(cal.PricesUSD))
		for date, usd := range cal.PricesUSD {
			if usd <= 0 {
				continue
			}
			uzs := utils.ConvertOperatorPriceToUzs(usd, rate)
			days[date] = tourCalendarDay{
				PriceUSD:     usd,
				PriceUZS:     uzs,
				PerPersonUZS: uzs / adults,
				PerPersonUSD: math.Round(usd/float64(adults)*100) / 100,
			}
		}
		c.Response().Header().Set("Cache-Control", "public, max-age=600")
		return c.JSON(http.StatusOK, tourCalendarResponse{
			Ready:      true,
			Adults:     adults,
			NightsFrom: cal.NightsFrom,
			NightsTo:   cal.NightsTo,
			UpdatedAt:  cal.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			Days:       days,
		})
	}
}
