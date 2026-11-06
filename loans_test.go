package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoansAreForwardedToLoansService(t *testing.T) {
	const body = `{"id":1,"bookId":7,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z"}`

	var method, path, received string
	loans := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading loan request body: %v", err)
		}
		received = string(raw)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, body)
	}))
	defer loans.Close()

	gateway := newRouterWithLoans("http://catalog.invalid", loans.URL)

	req := httptest.NewRequest(http.MethodPost, "/api/loans", strings.NewReader(`{"bookId":7}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body %q, want %q", got, body)
	}
	if method != http.MethodPost {
		t.Fatalf("loans service received method %q, want POST", method)
	}
	if path != "/loans" {
		t.Fatalf("loans service received path %q, want /loans", path)
	}
	if received != `{"bookId":7}` {
		t.Fatalf("loans service received body %q, want %q", received, `{"bookId":7}`)
	}
}

func TestLoansUnavailableIsBadGateway(t *testing.T) {
	loans := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := loans.URL
	loans.Close()

	gateway := newRouterWithLoans("http://catalog.invalid", closedURL)

	req := httptest.NewRequest(http.MethodPost, "/api/loans", strings.NewReader(`{"bookId":7}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"loans unavailable"}` {
		t.Fatalf("body %q, want %q", got, `{"error":"loans unavailable"}`)
	}
}

func TestLoansURLFromEnv(t *testing.T) {
	t.Setenv("LOANS_URL", "")
	if got := loansURLFromEnv(); got != "http://localhost:8082" {
		t.Fatalf("empty LOANS_URL resolved to %q, want http://localhost:8082", got)
	}

	t.Setenv("LOANS_URL", "http://loans.internal:9100")
	if got := loansURLFromEnv(); got != "http://loans.internal:9100" {
		t.Fatalf("LOANS_URL resolved to %q, want http://loans.internal:9100", got)
	}
}
