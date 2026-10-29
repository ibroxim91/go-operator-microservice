package handlers

import (
	"sync"

	"go-operator-service/models"
)

// Bir xil qidiruvlar bitta operator so'roviga birlashadi.
//
// Kesh natija tayyor bo'lgandan keyingina yoziladi. Ungacha bir xil
// qidiruvni boshlagan har bir mijoz operatorlarga o'z so'rovlarini
// yuborardi: reklamadan 100 kishi bir vaqtda "BAA" ni qidirsa — 100 marta.
// Endi birinchisi so'raydi, qolganlari uning natijasini kutadi.
//
// Natijani hech kim asl holicha ishlatmaydi: sahifalash chiptalarni
// saralaydi va ularga share token yozadi, shuning uchun har bir chaqiruvchi
// — birinchisi ham — o'z nusxasini oladi.

type searchCall struct {
	done   chan struct{}
	result *models.AsyncSamoResult
	err    error
}

var (
	searchCallsMu sync.Mutex
	searchCalls   = map[string]*searchCall{}
)

// coalesceSearch — `key` bo'yicha bir vaqtda faqat bitta `load` ishlaydi.
// Qaytgan qiymatdagi `shared` — natija boshqa chaqiruvdan olinganmi.
func coalesceSearch(key string, load func() (*models.AsyncSamoResult, error)) (result *models.AsyncSamoResult, shared bool, err error) {
	searchCallsMu.Lock()
	if call, ok := searchCalls[key]; ok {
		searchCallsMu.Unlock()
		<-call.done
		return copyAsyncResult(call.result), true, call.err
	}
	call := &searchCall{done: make(chan struct{})}
	searchCalls[key] = call
	searchCallsMu.Unlock()

	// load panika qilsa ham kutayotganlar qotib qolmasin.
	defer func() {
		searchCallsMu.Lock()
		delete(searchCalls, key)
		searchCallsMu.Unlock()
		close(call.done)
	}()
	call.result, call.err = load()
	return copyAsyncResult(call.result), false, call.err
}

func copyAsyncResult(src *models.AsyncSamoResult) *models.AsyncSamoResult {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Data.Results.Tickets = copyTickets(src.Data.Results.Tickets)
	dst.Data.Results.RecommendedTickets = copyTickets(src.Data.Results.RecommendedTickets)
	return &dst
}

func copyTickets(src []*models.Ticket) []*models.Ticket {
	if src == nil {
		return nil
	}
	dst := make([]*models.Ticket, len(src))
	for i, ticket := range src {
		if ticket == nil {
			continue
		}
		clone := *ticket
		dst[i] = &clone
	}
	return dst
}
