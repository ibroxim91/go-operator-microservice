package services

import (
	"fmt"
	"sort"
	"strings"

	"go-operator-service/models"
)

// hotelGroupKey — bitta kartaga tushadigan takliflarni aniqlaydi.
//
// Kelishuv: bitta tur = mehmonxona + zaezd sanasi + kecha soni. Sana boshqa
// bo'lsa — bu boshqa tur, shuning uchun alohida karta.
//
// Xona turi va ovqatlanish ataylab kalitga kirmaydi: talab bitta
// mehmonxonaga bitta karta, qolgan variantlar kartaning ichida ko'rinadi.
//
// Zaxira kalitlar ixtiyoriy emas. Mehmonxonalarning bir qismi mapping'siz
// keladi va `HotelDBID = 0` bo'ladi. Faqat shu maydon bo'yicha guruhlasak,
// butun davlatdagi noma'lum mehmonxonalar bitta kartaga yopishadi — testda
// to'g'ri ishlayotgandek ko'rinadigan xato. Shuning uchun pastda har
// bosqichda identifikator toraytiriladi, hech qachon kengaytirilmaydi.
func hotelGroupKey(ticket *models.Ticket) string {
	checkin := strings.TrimSpace(ticket.DepartureDate)
	if checkin == "" {
		checkin = strings.TrimSpace(ticket.DepartureTime)
	}
	suffix := fmt.Sprintf("|%s|%d", checkin, ticket.Nights)

	if ticket.HotelDBID > 0 {
		return fmt.Sprintf("db:%d%s", ticket.HotelDBID, suffix)
	}

	// Mapping yo'q: operatorning o'z id'si bilan guruhlaymiz. Bu bir
	// operator ichidagi xona/ovqat variantlarini birlashtiradi, lekin
	// operatorlar orasida birlashtirmaydi — chunki ular bir xil
	// mehmonxona ekanini aytadigan narsa yo'q.
	if len(ticket.TicketHotel) > 0 && ticket.TicketHotel[0].ID > 0 {
		return fmt.Sprintf("op:%s:%d%s", ticket.Operator, ticket.TicketHotel[0].ID, suffix)
	}

	// Ikkala id ham yo'q. Bu yerda taxmin qilish xavfli, shuning uchun
	// taklif o'z guruhida qoladi.
	return fmt.Sprintf("uniq:%s:%s", ticket.Operator, ticket.TourOperatorID)
}

// cheaperOffer — 0 yoki manfiy narx "eng arzon" bo'lib qolmasligi uchun.
// Ba'zi operatorlar narxni bo'sh qaytaradi va u 0 ga aylanadi.
func cheaperOffer(candidate, current *models.Ticket) bool {
	if current == nil {
		return true
	}
	if candidate.PriceFull <= 0 {
		return false
	}
	if current.PriceFull <= 0 {
		return true
	}
	return candidate.PriceFull < current.PriceFull
}

func buildTicketOffer(ticket *models.Ticket) models.TicketOffer {
	meal := ""
	if len(ticket.TicketHotel) > 0 {
		meal = ticket.TicketHotel[0].MealPlan
	}
	return models.TicketOffer{
		Operator:       ticket.Operator,
		TourOperatorID: ticket.TourOperatorID,
		PriceFull:      ticket.PriceFull,
		Price:          ticket.Price,
		RoomType:       ticket.RoomType,
		Meal:           meal,
		Nights:         ticket.Nights,
		DepartureDate:  ticket.DepartureDate,
	}
}

// existingOffers — allaqachon guruhlangan kartani qayta guruhlash uning
// takliflarini yo'qotmasligi uchun. Shusiz funksiya ikkinchi chaqiruvda
// kartani bitta taklifga tushirib qo'yardi.
func existingOffers(ticket *models.Ticket) []models.TicketOffer {
	if len(ticket.Offers) > 0 {
		return ticket.Offers
	}
	return []models.TicketOffer{buildTicketOffer(ticket)}
}

// GroupTicketsByHotel — har guruhdan eng arzon taklifni karta sifatida
// qoldiradi, qolganlarini o'sha kartaning ichiga yig'adi.
//
// Kirish tartibi saqlanadi: guruh birinchi marta uchragan joyda turadi.
// Saralashni chaqiruvchi o'zi qiladi.
//
// Funksiya idempotent: `Offers` har safar qaytadan quriladi, shuning uchun
// bir ro'yxatni ikki marta guruhlash takliflarni ikkilantirmaydi.
func GroupTicketsByHotel(tickets []*models.Ticket) []*models.Ticket {
	if len(tickets) == 0 {
		return tickets
	}

	type hotelGroup struct {
		winner *models.Ticket
		offers []models.TicketOffer
	}

	order := make([]string, 0, len(tickets))
	groups := make(map[string]*hotelGroup, len(tickets))

	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		key := hotelGroupKey(ticket)
		group, seen := groups[key]
		if !seen {
			group = &hotelGroup{}
			groups[key] = group
			order = append(order, key)
		}
		if cheaperOffer(ticket, group.winner) {
			group.winner = ticket
		}
		group.offers = append(group.offers, existingOffers(ticket)...)
	}

	out := make([]*models.Ticket, 0, len(order))
	for _, key := range order {
		group := groups[key]
		if group.winner == nil {
			continue
		}
		offers := dedupeOffers(group.offers)
		sort.SliceStable(offers, func(i, j int) bool {
			return offers[i].PriceFull < offers[j].PriceFull
		})
		group.winner.Offers = offers
		group.winner.OffersCount = len(offers)
		out = append(out, group.winner)
	}
	return out
}

// dedupeOffers bir xil taklifni ikki marta ko'rsatmaydi.
//
// Operator ba'zan aynan bir xil kombinatsiyani qayta qaytaradi: bir xil
// narx, xona, ovqatlanish va — eng muhimi — bir xil `tour_operator_id`.
// Bron identifikatori bir xil bo'lgani uchun bu haqiqatan bitta taklif,
// ikkita emas; mijoz kartani ochganda bir qatorni ikki marta ko'rardi.
//
// Noyoblashtirish aynan `tour_operator_id` bo'yicha: u bron qilinadigan
// narsaning o'zi. Narx yoki xona bo'yicha qilsak, bir xil ko'rinadigan
// lekin boshqa reysga tegishli ikki taklifni yo'qotib qo'yish xavfi
// bo'lardi. (O'lchandi: 246 taklifdan 31 tasi takror edi va hammasining
// bron id'si ham bir xil chiqdi.)
//
// Id'siz taklif — agar shunday bo'lsa — qoldiriladi: uni tashlab yuborish
// bron qilinadigan variantni yo'qotishi mumkin.
func dedupeOffers(offers []models.TicketOffer) []models.TicketOffer {
	seen := make(map[string]struct{}, len(offers))
	out := make([]models.TicketOffer, 0, len(offers))
	for _, offer := range offers {
		if offer.TourOperatorID == "" {
			out = append(out, offer)
			continue
		}
		if _, ok := seen[offer.TourOperatorID]; ok {
			continue
		}
		seen[offer.TourOperatorID] = struct{}{}
		out = append(out, offer)
	}
	return out
}
