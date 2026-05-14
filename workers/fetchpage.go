package workers
import (
	"context"
	"encoding/json"
	"fmt"
	"go-operator-service/models"
	"go-operator-service/services"
	"go-operator-service/utils"

	"io"
	"log"
	"net/http"

)



func FetchPage(ctx context.Context, page int, job models.Request, hotelService *services.HotelService, ch chan<- []*models.Ticket) {
    url := fmt.Sprintf("%s&PRICEPAGE=%d", job.Url, page)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    resp, err := http.DefaultClient.Do(req)
	log.Println("Start fetch page ", page, " for operator ", job.Operator)
    if err != nil {
        log.Println("Error fetching page", page, err)
        ch <- nil
        return
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    var parsed models.SearchTourResponse
    if err := json.Unmarshal(body, &parsed); err != nil {
        log.Println("Error parsing page", page, err)
        ch <- nil
        return
    }

    tickets := []*models.Ticket{}
    for _, price := range parsed.SearchTour_PRICES.Prices {
        if price.FreightExternal == "Y"  {
            continue
        }
        ticket := utils.TransformSamoPriceToTicket(
            price, job.Departure,
            job.Operator, job.DestCountryName, job.DestImageUrl,
            job.CurrentUsdCourse, job.DestinationID,
            job.DepartureID, job.CountryID, hotelService, false,
        )
        tickets = append(tickets, ticket)
    }
    ch <- tickets
}