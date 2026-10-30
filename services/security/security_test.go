package security_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"nfl-quiniela-2026/services/security"
)

func TestClientIPResolver_PreventsSpoofing(t *testing.T) {
	// Resolver with only loopback (127.0.0.1) as trusted
	resolver := security.NewClientIPResolver([]string{"127.0.0.1"}, false)

	// Case 1: Untrusted connection from internet (203.0.113.50) attempting to spoof via X-Forwarded-For
	reqDirect := httptest.NewRequest("GET", "/picks", nil)
	reqDirect.RemoteAddr = "203.0.113.50:54321"
	reqDirect.Header.Set("X-Forwarded-For", "198.51.100.22, 10.0.0.1")
	reqDirect.Header.Set("X-Real-IP", "1.1.1.1")

	ip := resolver.ResolveClientIP(reqDirect)
	if ip != "203.0.113.50" {
		t.Fatalf("Expected direct connection IP 203.0.113.50 to be used (ignoring spoofed headers), got: %s", ip)
	}

	// Case 2: Connection from trusted proxy (127.0.0.1) forwarding legitimate client IP
	reqProxy := httptest.NewRequest("GET", "/picks", nil)
	reqProxy.RemoteAddr = "127.0.0.1:45678"
	reqProxy.Header.Set("X-Forwarded-For", "203.0.113.77, 127.0.0.1")

	ipProxy := resolver.ResolveClientIP(reqProxy)
	if ipProxy != "203.0.113.77" {
		t.Fatalf("Expected proxy client IP 203.0.113.77, got: %s", ipProxy)
	}
}

func TestCSRFProtection_BlocksCrossSite(t *testing.T) {
	mw := security.CSRFProtectionMiddleware("https://quiniela.cmoreno.org")
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := mw(dummyHandler)

	// Case 1: GET request from anywhere passes
	reqGet := httptest.NewRequest("GET", "/picks", nil)
	reqGet.Header.Set("Origin", "https://malicious-site.com")
	rrGet := httptest.NewRecorder()
	handler.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("Expected GET to pass CSRF check, got status: %d", rrGet.Code)
	}

	// Case 2: POST with cross-site Origin blocked
	reqPostEvil := httptest.NewRequest("POST", "/picks/save", nil)
	reqPostEvil.Host = "quiniela.cmoreno.org"
	reqPostEvil.Header.Set("Origin", "https://malicious-attacker.com")
	rrPostEvil := httptest.NewRecorder()
	handler.ServeHTTP(rrPostEvil, reqPostEvil)
	if rrPostEvil.Code != http.StatusForbidden {
		t.Fatalf("Expected cross-site POST to be blocked with 403, got: %d", rrPostEvil.Code)
	}

	// Case 3: POST with Sec-Fetch-Site: cross-site blocked
	reqSecFetch := httptest.NewRequest("POST", "/picks/save", nil)
	reqSecFetch.Host = "quiniela.cmoreno.org"
	reqSecFetch.Header.Set("Sec-Fetch-Site", "cross-site")
	rrSecFetch := httptest.NewRecorder()
	handler.ServeHTTP(rrSecFetch, reqSecFetch)
	if rrSecFetch.Code != http.StatusForbidden {
		t.Fatalf("Expected Sec-Fetch-Site: cross-site to be blocked with 403, got: %d", rrSecFetch.Code)
	}

	// Case 4: Legitimate same-origin POST succeeds
	reqPostGood := httptest.NewRequest("POST", "/picks/save", nil)
	reqPostGood.Host = "quiniela.cmoreno.org"
	reqPostGood.Header.Set("Origin", "https://quiniela.cmoreno.org")
	rrPostGood := httptest.NewRecorder()
	handler.ServeHTTP(rrPostGood, reqPostGood)
	if rrPostGood.Code != http.StatusOK {
		t.Fatalf("Expected same-origin POST to succeed with 200, got: %d", rrPostGood.Code)
	}
}

func TestIPRateLimiter_ThrottlesBursts(t *testing.T) {
	// Limiter with 60 req/min (1 req/sec) and burst capacity of 3
	limiter := security.NewIPRateLimiter(60, 3)
	defer limiter.Stop()

	testIP := "192.0.2.1"

	// First 3 requests (burst) should pass
	for i := 1; i <= 3; i++ {
		allowed, _ := limiter.Allow(testIP)
		if !allowed {
			t.Fatalf("Expected burst request %d to be allowed", i)
		}
	}

	// 4th request immediately should be rejected
	allowed, waitTime := limiter.Allow(testIP)
	if allowed {
		t.Fatalf("Expected 4th request in rapid succession to be rejected")
	}
	if waitTime <= 0 {
		t.Fatalf("Expected positive wait duration, got: %v", waitTime)
	}

	// Different IP should still have its own tokens
	allowedOther, _ := limiter.Allow("192.0.2.2")
	if !allowedOther {
		t.Fatalf("Expected independent IP to be allowed")
	}
}

func TestAuthRateLimitMiddleware_BlocksAfterLimit(t *testing.T) {
	limiter := security.NewIPRateLimiter(60, 2)
	defer limiter.Stop()

	mw := security.AuthRateLimitMiddleware(limiter)
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(dummyHandler)

	ip := "198.51.100.99"

	// Request 1 and 2
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		ctx := context.WithValue(req.Context(), security.ClientIPContextKey, ip)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req.WithContext(ctx))
		if rr.Code != http.StatusOK {
			t.Fatalf("Request %d expected 200, got %d", i+1, rr.Code)
		}
	}

	// Request 3 should be 429
	req := httptest.NewRequest("POST", "/login", nil)
	ctx := context.WithValue(req.Context(), security.ClientIPContextKey, ip)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req.WithContext(ctx))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected HTTP 429 Too Many Requests, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatalf("Expected Retry-After header to be set")
	}
}
