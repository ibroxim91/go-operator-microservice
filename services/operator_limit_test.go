package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newRequest(t *testing.T, ctx context.Context, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestDoOperatorNeverExceedsLimit(t *testing.T) {
	t.Setenv("OPERATOR_MAX_CONCURRENCY", "3")
	var inFlight, peak int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&peak)
			if now <= old || atomic.CompareAndSwapInt32(&peak, old, now) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body, err := DoOperator(server.Client(), "limit-test", newRequest(t, context.Background(), server.URL))
			if err != nil || status != 200 || string(body) != `{"ok":true}` {
				t.Errorf("status=%d body=%q err=%v", status, body, err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&peak); got > 3 {
		t.Fatalf("operatorga bir vaqtda %d so'rov ketdi, chegara 3", got)
	} else if got < 2 {
		t.Fatalf("parallellik yo'qolgan: eng ko'pi %d", got)
	}
}

func TestDoOperatorGivesUpWhenSlotsStayBusy(t *testing.T) {
	t.Setenv("OPERATOR_MAX_CONCURRENCY", "1")
	old := operatorSlotWait
	operatorSlotWait = 50 * time.Millisecond
	defer func() { operatorSlotWait = old }()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	go func() {
		_, _, _ = DoOperator(server.Client(), "busy-test", newRequest(t, context.Background(), server.URL))
	}()
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	_, _, err := DoOperator(server.Client(), "busy-test", newRequest(t, context.Background(), server.URL))
	if !errors.Is(err, ErrOperatorBusy) {
		t.Fatalf("ErrOperatorBusy kutilgan, chiqdi: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("band slotni kutish chegaradan uzoq davom etdi")
	}
}

func TestDoOperatorStopsWaitingWhenRequestIsCancelled(t *testing.T) {
	t.Setenv("OPERATOR_MAX_CONCURRENCY", "1")
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer server.Close()
	defer close(release)

	go func() {
		_, _, _ = DoOperator(server.Client(), "cancel-test", newRequest(t, context.Background(), server.URL))
	}()
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, _, err := DoOperator(server.Client(), "cancel-test", newRequest(t, ctx, server.URL))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context xatosi kutilgan, chiqdi: %v", err)
	}
}

func TestDoOperatorReleasesSlotAfterFailure(t *testing.T) {
	t.Setenv("OPERATOR_MAX_CONCURRENCY", "1")
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	for i := 0; i < 3; i++ {
		if _, _, err := DoOperator(http.DefaultClient, "failure-test", newRequest(t, context.Background(), deadURL)); err == nil {
			t.Fatal("yopiq serverga so'rov xato berishi kerak")
		}
	}
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer live.Close()
	if _, body, err := DoOperator(live.Client(), "failure-test", newRequest(t, context.Background(), live.URL)); err != nil || string(body) != "ok" {
		t.Fatalf("xatolardan keyin slot bo'shamagan: %v", err)
	}
}

func TestOperatorsDoNotWaitForEachOther(t *testing.T) {
	t.Setenv("OPERATOR_MAX_CONCURRENCY", "1")
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer slow.Close()
	defer close(release)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer fast.Close()

	go func() {
		_, _, _ = DoOperator(slow.Client(), "slow-operator", newRequest(t, context.Background(), slow.URL))
	}()
	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	if _, _, err := DoOperator(fast.Client(), "fast-operator", newRequest(t, context.Background(), fast.URL)); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("boshqa operatorning band sloti kutildi")
	}
}

func TestBasePagesKeepSlotsWhileExtraPagesAreBusy(t *testing.T) {
	t.Setenv("OPERATOR_MAX_CONCURRENCY", "4")
	release := make(chan struct{})
	var extraInFlight, extraPeak int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("p") == "extra" {
			now := atomic.AddInt32(&extraInFlight, 1)
			for {
				old := atomic.LoadInt32(&extraPeak)
				if now <= old || atomic.CompareAndSwapInt32(&extraPeak, old, now) {
					break
				}
			}
			<-release
			atomic.AddInt32(&extraInFlight, -1)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	// 10 ta qo'shimcha sahifa (10+) bir vaqtda — slotlarning yarmidan ko'pini olmasligi kerak
	for i := 0; i < 10; i++ {
		go func() {
			_, _, _ = DoOperatorPage(server.Client(), "priority-test", newRequest(t, context.Background(), server.URL+"?p=extra"), 15)
		}()
	}
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	if _, body, err := DoOperatorPage(server.Client(), "priority-test", newRequest(t, context.Background(), server.URL+"?p=base"), 3); err != nil || string(body) != "ok" {
		t.Fatalf("3-sahifa joy topmadi: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("3-sahifa qo'shimcha sahifalarni kutib qoldi")
	}
	close(release)
	if got := atomic.LoadInt32(&extraPeak); got > 2 {
		t.Fatalf("qo'shimcha sahifalar bir vaqtda %d slot oldi, 2 dan oshmasligi kerak", got)
	}
}
