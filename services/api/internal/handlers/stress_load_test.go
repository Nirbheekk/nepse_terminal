package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nepse/terminal/api/internal/services"
)

func TestRateLimiterHighConcurrencyBurst(t *testing.T) {
	// 60 RPM = 1 request per second sustained, with burst of 60
	limiter := newLimiterMap(60)

	h := rateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const totalRequests = 100
	var successCount int64
	var rateLimitedCount int64

	var wg sync.WaitGroup
	wg.Add(totalRequests)

	for i := 0; i < totalRequests; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/market/summary", nil)
			req.RemoteAddr = "192.168.1.50:12345" // Same IP to trigger bucket exhaustion
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code == http.StatusOK {
				atomic.AddInt64(&successCount, 1)
			} else if rr.Code == http.StatusTooManyRequests {
				atomic.AddInt64(&rateLimitedCount, 1)
			}
		}()
	}
	wg.Wait()

	if successCount == 0 {
		t.Errorf("expected some requests to succeed within burst, got 0")
	}
	if rateLimitedCount == 0 {
		t.Errorf("expected rate limiter to reject excess requests beyond burst of 60, but got 0 429s (success: %d)", successCount)
	}
	if successCount+rateLimitedCount != totalRequests {
		t.Errorf("unexpected status distribution: %d successes, %d limited", successCount, rateLimitedCount)
	}
}

func TestLimiterMapIPChurnAndEviction(t *testing.T) {
	limiter := newLimiterMap(100)

	// Simulate 1000 unique client IPs
	for i := 0; i < 1000; i++ {
		ip := fmt.Sprintf("10.0.%d.%d", i/256, i%256)
		if !limiter.allow(ip) {
			t.Fatalf("first request for fresh IP %s should be allowed", ip)
		}
	}

	limiter.mu.Lock()
	countBefore := len(limiter.buckets)
	limiter.mu.Unlock()

	if countBefore != 1000 {
		t.Fatalf("expected 1000 tracked buckets, got %d", countBefore)
	}

	// Manually simulate eviction cutoff
	cutoff := time.Now().Add(1 * time.Minute) // everything seen before this cutoff
	limiter.mu.Lock()
	for k, b := range limiter.buckets {
		if b.seen.Before(cutoff) {
			delete(limiter.buckets, k)
		}
	}
	countAfter := len(limiter.buckets)
	limiter.mu.Unlock()

	if countAfter != 0 {
		t.Errorf("expected all idle buckets to be evicted, got %d remaining", countAfter)
	}
}

func TestDeveloperKeyQuotaConcurrency(t *testing.T) {
	key := &services.DeveloperKey{
		ID:                "key-load-1",
		UserID:            "usr-1",
		MonthlyQuota:      50,
		RequestsThisMonth: 0,
		QuotaResetAt:      time.Now().UTC().Add(24 * time.Hour),
	}

	var mu sync.Mutex
	const concurrentCallers = 80
	var successCount int64
	var quotaExceededCount int64

	var wg sync.WaitGroup
	wg.Add(concurrentCallers)

	for i := 0; i < concurrentCallers; i++ {
		go func() {
			defer wg.Done()
			mu.Lock()
			err := key.RecordRequest()
			mu.Unlock()

			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&quotaExceededCount, 1)
			}
		}()
	}
	wg.Wait()

	if successCount != 50 {
		t.Errorf("expected exactly 50 allowed calls under quota, got %d", successCount)
	}
	if quotaExceededCount != 30 {
		t.Errorf("expected exactly 30 rejected calls above quota, got %d", quotaExceededCount)
	}
}
