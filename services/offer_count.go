package services

import "go-operator-service/models"

// CountOffers — kartalardagi takliflar yig'indisi. Bitta karta bir
// mehmonxonaning bir nechta taklifini birlashtiradi (OffersCount);
// guruhlanmagan karta — bitta taklif. Sahifa sarlavhasida kartalar emas,
// mana shu son ko'rsatiladi.
func CountOffers(tickets []*models.Ticket) int {
	total := 0
	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		if ticket.OffersCount > 0 {
			total += ticket.OffersCount
		} else {
			total++
		}
	}
	return total
}
