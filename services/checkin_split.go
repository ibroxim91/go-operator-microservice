package services

import (
	"net/url"
	"os"
	"strconv"
	"time"

	"go-operator-service/logger"
	"go-operator-service/models"
)

// Uzun sana oralig'ini operator juda sekin qidiradi: Kompas'da Dubay
// 2–21 noyabr bitta sahifasi 25–30 s, xuddi shu oraliq 7 kunlik
// bo'laklarda 4–7 s. Javob kutish chegarasidan oshib ketgani uchun bunday
// qidiruv "hech narsa topilmadi" bo'lib qolardi. Shuning uchun uzun oraliq
// bo'laklarga bo'linadi va har bo'lak alohida, parallel so'rov bo'ladi.
//
// Odatiy 7 kunlik qidiruv (masalan 3–10) bo'linmaydi.
const (
	defaultCheckinChunkDays = 7 // OPERATOR_CHECKIN_CHUNK_DAYS; 0 — bo'lish o'chiq
	maxCheckinChunks        = 6
	samoDateLayout          = "20060102"
)

func checkinChunkDays() int {
	if raw, ok := os.LookupEnv("OPERATOR_CHECKIN_CHUNK_DAYS"); ok {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			return n
		}
	}
	return defaultCheckinChunkDays
}

// SplitLongCheckinJobs uzun oraliqli so'rovlarni bo'laklarga ajratadi.
// Bo'laklar birgalikda bitta so'rov oladigan sahifa chegarasidan ko'p
// sahifa olmaydi — operatorga yuk oshmaydi.
func SplitLongCheckinJobs(jobs []models.Request) []models.Request {
	days := checkinChunkDays()
	if days <= 0 {
		return jobs
	}

	out := make([]models.Request, 0, len(jobs))
	for _, job := range jobs {
		urls := splitCheckinURL(job.Url, days)
		if len(urls) <= 1 {
			out = append(out, job)
			continue
		}

		pages := pagesPerChunk(len(urls))
		for _, u := range urls {
			chunk := job
			chunk.Url = u
			chunk.MaxPages = pages
			out = append(out, chunk)
		}
		logger.Log.Info().
			Str("handler", "async-samo/tickets").
			Str("operator", job.Operator).
			Int("chunks", len(urls)).
			Int("max_pages_per_chunk", pages).
			Msg("long check-in range split")
	}
	return out
}

// splitCheckinURL CHECKIN_BEG..CHECKIN_END oralig'ini bo'laklarga bo'lib,
// har bo'lak uchun URL qaytaradi. Oraliq qisqa yoki o'qib bo'lmasa —
// asl URL o'zi.
func splitCheckinURL(rawURL string, days int) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return []string{rawURL}
	}
	q := u.Query()
	beg, err1 := time.Parse(samoDateLayout, q.Get("CHECKIN_BEG"))
	end, err2 := time.Parse(samoDateLayout, q.Get("CHECKIN_END"))
	if err1 != nil || err2 != nil || end.Before(beg) {
		return []string{rawURL}
	}

	span := int(end.Sub(beg).Hours() / 24)
	if span <= days {
		return []string{rawURL}
	}

	// Bo'lak: boshlanishdan keyingi `days` kun ham kiradi (3–10 kabi).
	size := days + 1
	total := span + 1
	if (total+size-1)/size > maxCheckinChunks {
		size = (total + maxCheckinChunks - 1) / maxCheckinChunks
	}

	var urls []string
	for start := beg; !start.After(end); start = start.AddDate(0, 0, size) {
		stop := start.AddDate(0, 0, size-1)
		if stop.After(end) {
			stop = end
		}
		q.Set("CHECKIN_BEG", start.Format(samoDateLayout))
		q.Set("CHECKIN_END", stop.Format(samoDateLayout))
		chunk := *u
		chunk.RawQuery = q.Encode()
		urls = append(urls, chunk.String())
	}
	return urls
}

// pagesPerChunk — bitta so'rovning sahifa chegarasi bo'laklar o'rtasida
// bo'linadi, lekin har bo'lak kamida birinchi BasePages sahifani oladi.
func pagesPerChunk(chunks int) int {
	limit := OperatorPagesToFetch(1 << 30)
	per := (limit + chunks - 1) / chunks
	if per < BasePages {
		per = BasePages
	}
	return per
}
