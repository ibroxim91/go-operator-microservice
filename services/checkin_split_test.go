package services

import (
	"net/url"
	"testing"
	"time"

	"go-operator-service/models"
)

func checkinRanges(t *testing.T, urls []string) [][2]string {
	t.Helper()
	out := make([][2]string, 0, len(urls))
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		q := u.Query()
		out = append(out, [2]string{q.Get("CHECKIN_BEG"), q.Get("CHECKIN_END")})
	}
	return out
}

func samoURL(beg, end string) string {
	return "https://online.example.uz/export/default.php?ADULT=2&CHECKIN_BEG=" + beg +
		"&CHECKIN_END=" + end + "&PRICEPAGE=1&STATEINC=23&TOWNS=459%2C413"
}

func TestSplitCheckinURLKeepsUsualWeek(t *testing.T) {
	for _, tc := range [][2]string{
		{"20261105", "20261112"}, // odatiy 7 kun (3..10)
		{"20261105", "20261105"}, // bitta kun
		{"20261105", "20261108"},
	} {
		got := splitCheckinURL(samoURL(tc[0], tc[1]), 7)
		if len(got) != 1 || got[0] != samoURL(tc[0], tc[1]) {
			t.Fatalf("%v: bo'linmasligi kerak edi, %v", tc, got)
		}
	}
}

func TestSplitCheckinURLBossCase(t *testing.T) {
	// 2–21 noyabr: operatorda 25–30 s edi.
	got := checkinRanges(t, splitCheckinURL(samoURL("20261102", "20261121"), 7))
	want := [][2]string{
		{"20261102", "20261109"},
		{"20261110", "20261117"},
		{"20261118", "20261121"},
	}
	if len(got) != len(want) {
		t.Fatalf("bo'laklar: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bo'lak %d: %v, kutilgan %v", i, got[i], want[i])
		}
	}
}

func TestSplitCheckinURLCoversRangeWithoutGapsOrOverlap(t *testing.T) {
	for _, tc := range [][2]string{
		{"20261102", "20261110"}, // 8 kun — ikki bo'lak
		{"20261102", "20261203"}, // 31 kun
		{"20261102", "20270127"}, // 86 kun — bo'laklar soni cheklanadi
		{"20261228", "20270110"}, // yil almashadi
	} {
		urls := splitCheckinURL(samoURL(tc[0], tc[1]), 7)
		if len(urls) > maxCheckinChunks {
			t.Fatalf("%v: %d bo'lak, chegara %d", tc, len(urls), maxCheckinChunks)
		}
		ranges := checkinRanges(t, urls)
		if ranges[0][0] != tc[0] || ranges[len(ranges)-1][1] != tc[1] {
			t.Fatalf("%v: chetlar mos emas: %v", tc, ranges)
		}
		for i := 1; i < len(ranges); i++ {
			prevEnd, _ := time.Parse(samoDateLayout, ranges[i-1][1])
			start, _ := time.Parse(samoDateLayout, ranges[i][0])
			if start.Sub(prevEnd).Hours() != 24 {
				t.Fatalf("%v: bo'shliq yoki ustma-ust: %v", tc, ranges)
			}
		}
	}
}

func TestSplitCheckinURLKeepsOtherParams(t *testing.T) {
	urls := splitCheckinURL(samoURL("20261102", "20261121"), 7)
	for _, raw := range urls {
		u, _ := url.Parse(raw)
		q := u.Query()
		if u.Host != "online.example.uz" || u.Path != "/export/default.php" ||
			q.Get("ADULT") != "2" || q.Get("STATEINC") != "23" || q.Get("TOWNS") != "459,413" || q.Get("PRICEPAGE") != "1" {
			t.Fatalf("boshqa parametrlar o'zgardi: %s", raw)
		}
	}
}

func TestSplitCheckinURLBadInputUnchanged(t *testing.T) {
	for _, raw := range []string{
		samoURL("", "20261121"),
		samoURL("20261121", "20261102"), // teskari
		samoURL("2026-11-02", "2026-11-21"),
		"https://online.example.uz/export/default.php?ADULT=2",
	} {
		got := splitCheckinURL(raw, 7)
		if len(got) != 1 || got[0] != raw {
			t.Fatalf("%q o'zgarmasligi kerak edi: %v", raw, got)
		}
	}
}

func TestSplitLongCheckinJobs(t *testing.T) {
	t.Setenv("OPERATOR_MAX_PAGES", "60")
	jobs := []models.Request{
		{Url: samoURL("20261102", "20261121"), Operator: "samo_tour", Page: 1},
		{Url: samoURL("20261105", "20261112"), Operator: "easy_booking", Page: 1},
	}
	t.Setenv("OPERATOR_CHECKIN_CHUNK_DAYS", "7")
	got := SplitLongCheckinJobs(jobs)
	if len(got) != 4 {
		t.Fatalf("4 ta so'rov kutilgan edi (3 bo'lak + 1), %d", len(got))
	}
	for _, j := range got[:3] {
		if j.Operator != "samo_tour" || j.Page != 1 || j.MaxPages != 20 {
			t.Fatalf("bo'lak noto'g'ri: %+v", j)
		}
	}
	if got[3].Url != jobs[1].Url || got[3].MaxPages != 0 {
		t.Fatalf("odatiy so'rov o'zgarmasligi kerak: %+v", got[3])
	}
	if jobs[0].MaxPages != 0 {
		t.Fatalf("kirish massivi o'zgarmasin")
	}
}

func TestSplitLongCheckinJobsKillSwitch(t *testing.T) {
	t.Setenv("OPERATOR_CHECKIN_CHUNK_DAYS", "0")
	jobs := []models.Request{{Url: samoURL("20261102", "20261121"), Operator: "samo_tour"}}
	got := SplitLongCheckinJobs(jobs)
	if len(got) != 1 || got[0].Url != jobs[0].Url {
		t.Fatalf("0 bo'lganda bo'linmasligi kerak: %v", got)
	}
}

func TestPagesPerChunk(t *testing.T) {
	t.Setenv("OPERATOR_MAX_PAGES", "60")
	for chunks, want := range map[int]int{2: 30, 3: 20, 6: 10} {
		if got := pagesPerChunk(chunks); got != want {
			t.Fatalf("%d bo'lak: %d, kutilgan %d", chunks, got, want)
		}
	}
	t.Setenv("OPERATOR_MAX_PAGES", "20")
	if got := pagesPerChunk(6); got != BasePages {
		t.Fatalf("kamida BasePages: %d", got)
	}
}
