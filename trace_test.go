package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyCarriesRequestIDUpstream(t *testing.T) {
	var received string
	loans := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("X-Request-Id")

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	defer loans.Close()

	gateway := newRouterWithLoans("http://catalog.invalid", loans.URL)

	req := httptest.NewRequest(http.MethodPost, "/api/loans", strings.NewReader(`{"bookId":7}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "abc-123")
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if received != "abc-123" {
		t.Fatalf("loans service received X-Request-Id %q, want %q", received, "abc-123")
	}
}

func TestProxyReturnsUpstreamRequestID(t *testing.T) {
	loans := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "abc-123")
		_, _ = io.WriteString(w, `[]`)
	}))
	defer loans.Close()

	gateway := newRouterWithLoans("http://catalog.invalid", loans.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/loans", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "abc-123" {
		t.Fatalf("browser received X-Request-Id %q, want %q", got, "abc-123")
	}
	if got := rec.Body.String(); got != `[]` {
		t.Fatalf("body %q, want %q", got, `[]`)
	}
}

func TestProxySendsNoRequestIDWhenNoneGiven(t *testing.T) {
	var received string
	var present bool
	loans := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["X-Request-Id"]
		received = r.Header.Get("X-Request-Id")

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	defer loans.Close()

	gateway := newRouterWithLoans("http://catalog.invalid", loans.URL)

	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if present {
		t.Fatalf("loans service received an X-Request-Id header %q, want none", received)
	}
	if received != "" {
		t.Fatalf("loans service received X-Request-Id %q, want no header", received)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "" {
		t.Fatalf("browser received X-Request-Id %q, want no header", got)
	}
}
