package services

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Operatorga bir vaqtda ketadigan so'rovlar chegarasi — barcha qidiruvlar
// uchun umumiy. Ilgari har qidiruv o'zicha 9 tagacha sahifani parallel
// so'rardi va qidiruvlar soni ko'paysa operatorga yuk cheksiz o'sardi
// (oddiy kunlarda ham easy_booking 3 kunda 75 marta timeout bergan).
// Chegara bitta qidiruvning o'z parallelligidan kam emas, shuning uchun
// yolg'iz qidiruv sekinlashmaydi — faqat bir vaqtdagi qidiruvlar navbat
// kutadi.
//
// Yuk sinovida (soxta operator, 30 ta turli qidiruv bir vaqtda): chegara
// 16 natijalarning 59% ini kesdi, 32 esa eski kodning 87% ini berdi
// (o'sha sinovda eski kod operatorga bir vaqtda ~55, 100 ta bir xil
// qidiruvda ~250 so'rov yuborgan). OPERATOR_MAX_CONCURRENCY bilan sozlanadi.
const defaultOperatorConcurrency = 32

// 1-sahifa — eng arzon narxlar: usiz qidiruvda operator umuman bo'lmaydi,
// shuning uchun u slotni uzoqroq kutadi. Chuqurroq sahifalar yuk ostida
// qisqa kutib, o'tkazib yuboriladi — qidiruv qotib qolmasligi uchun.
var (
	operatorSlotWait         = 45 * time.Second
	operatorDeepPageSlotWait = 20 * time.Second
)

var ErrOperatorBusy = errors.New("operator busy: request slot not available")

var (
	operatorSlotsMu sync.Mutex
	operatorSlots   = map[string]chan struct{}{}
	extraPageSlots  = map[string]chan struct{}{}
)

// BasePages — eski kod oladigan sahifalar (1–9). Ulardan keyingi "qo'shimcha"
// sahifalar operator slotlarining ko'pi bilan yarmini ishlatadi: yuk ostida
// 1–9-sahifalar doim joy topadi va natija eski koddan kam bo'lmaydi
// (yuk sinovida usiz 30 ta turli qidiruvda kartalar 423 dan 279 ga tushgan).
const BasePages = 9

var extraPageSlotWait = 5 * time.Second

// Bitta qidiruvda operatordan olinadigan eng ko'p sahifa — operator
// noto'g'ri pager qaytarsa ham so'rovlar soni cheklansin. UAE: 30–37 sahifa.
const defaultOperatorMaxPages = 60

// OperatorPagesToFetch — operator aytgan sahifalar sonidan nechtasini olish.
// OPERATOR_MAX_PAGES bilan sozlanadi.
func OperatorPagesToFetch(total int) int {
	limit := defaultOperatorMaxPages
	if n, err := strconv.Atoi(os.Getenv("OPERATOR_MAX_PAGES")); err == nil && n > 0 {
		limit = n
	}
	if total > limit {
		return limit
	}
	return total
}

// OPERATOR_DEEP_PAGE_WAIT_SECONDS — chuqur sahifalar slotni qancha kutadi.
func deepPageSlotWait() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("OPERATOR_DEEP_PAGE_WAIT_SECONDS")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return operatorDeepPageSlotWait
}

func operatorConcurrency() int {
	if n, err := strconv.Atoi(os.Getenv("OPERATOR_MAX_CONCURRENCY")); err == nil && n > 0 {
		return n
	}
	return defaultOperatorConcurrency
}

func extraPageSlotsFor(operator string) chan struct{} {
	operatorSlotsMu.Lock()
	defer operatorSlotsMu.Unlock()
	slots, ok := extraPageSlots[operator]
	if !ok {
		size := operatorConcurrency() / 2
		if size < 1 {
			size = 1
		}
		slots = make(chan struct{}, size)
		extraPageSlots[operator] = slots
	}
	return slots
}

func operatorSlotsFor(operator string) chan struct{} {
	operatorSlotsMu.Lock()
	defer operatorSlotsMu.Unlock()
	slots, ok := operatorSlots[operator]
	if !ok {
		slots = make(chan struct{}, operatorConcurrency())
		operatorSlots[operator] = slots
	}
	return slots
}

// DoOperator — operatorga so'rovni umumiy chegara ostida yuboradi va javobni
// to'liq o'qiydi. Slot javob o'qilishi bilan bo'shaydi: chaqiruvchi keyin
// boshqa sahifalarni kutsa ham slot band turmaydi.
func DoOperator(client *http.Client, operator string, req *http.Request) (int, []byte, error) {
	return doOperator(client, operator, req, operatorSlotWait)
}

// DoOperatorDeepPage — 2-sahifadan keyingilar uchun: slot qisqa kutiladi.
func DoOperatorDeepPage(client *http.Client, operator string, req *http.Request) (int, []byte, error) {
	return doOperator(client, operator, req, deepPageSlotWait())
}

// DoOperatorPage — sahifa raqamiga qarab: 1–9 slotni uzoq kutadi (eski kod
// olgan natija hech qachon kamaymasin), 10+ esa avval "qo'shimcha"
// slotlardan joy oladi va yuk ostida birinchi bo'lib o'tkazib yuboriladi.
func DoOperatorPage(client *http.Client, operator string, req *http.Request, page int) (int, []byte, error) {
	switch {
	case page <= 1:
		return doOperator(client, operator, req, operatorSlotWait)
	case page <= BasePages:
		// 1–9 eski kod olgan qism: yuk ostida ham o'tkazib yuborilmasin.
		return doOperator(client, operator, req, operatorSlotWait)
	}
	extra := extraPageSlotsFor(operator)
	timer := time.NewTimer(extraPageSlotWait)
	defer timer.Stop()
	select {
	case extra <- struct{}{}:
	case <-req.Context().Done():
		return 0, nil, req.Context().Err()
	case <-timer.C:
		return 0, nil, ErrOperatorBusy
	}
	defer func() { <-extra }()
	return doOperator(client, operator, req, extraPageSlotWait)
}

func doOperator(client *http.Client, operator string, req *http.Request, wait time.Duration) (int, []byte, error) {
	slots := operatorSlotsFor(operator)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case slots <- struct{}{}:
	case <-req.Context().Done():
		return 0, nil, req.Context().Err()
	case <-timer.C:
		return 0, nil, ErrOperatorBusy
	}
	defer func() { <-slots }()

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, RedactError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, RedactError(err)
	}
	return resp.StatusCode, body, nil
}
