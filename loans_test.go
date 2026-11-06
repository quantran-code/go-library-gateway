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

func TestLoanReturnIsForwardedToLoansService(t *testing.T) {
	const body = `{"id":3,"bookId":7,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z","returnedAt":"2024-05-10T09:00:00Z"}`

	var methods, paths []string
	loans := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer loans.Close()

	gateway := newRouterWithLoans("http://catalog.invalid", loans.URL)

	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/loans/3/return", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body %q, want %q", got, body)
	}
	if len(methods) != 1 || methods[0] != http.MethodPost {
		t.Fatalf("loans service received methods %v, want [POST]", methods)
	}
	if len(paths) != 1 || paths[0] != "/loans/3/return" {
		t.Fatalf("loans service received paths %v, want [/loans/3/return]", paths)
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "/api") {
			t.Fatalf("loans service received path %q, the /api prefix was not stripped", p)
		}
	}
}

func TestLoanReturnStatusesPassThrough(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "returned",
			status: http.StatusOK,
			body:   `{"id":3,"bookId":7,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z","returnedAt":"2024-05-10T09:00:00Z"}`,
		},
		{name: "unknown loan", status: http.StatusNotFound, body: `{"error":"loan not found"}`},
		{name: "already returned", status: http.StatusConflict, body: `{"error":"loan already returned"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loans := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer loans.Close()

			gateway := newRouterWithLoans("http://catalog.invalid", loans.URL)

			rec := httptest.NewRecorder()
			gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/loans/3/return", nil))

			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tc.body {
				t.Fatalf("body %q, want %q", got, tc.body)
			}
		})
	}

	t.Run("loans unavailable", func(t *testing.T) {
		loans := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		closedURL := loans.URL
		loans.Close()

		gateway := newRouterWithLoans("http://catalog.invalid", closedURL)

		rec := httptest.NewRecorder()
		gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/loans/3/return", nil))

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status %d, want %d", rec.Code, http.StatusBadGateway)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"loans unavailable"}` {
			t.Fatalf("body %q, want %q", got, `{"error":"loans unavailable"}`)
		}
	})
}

func TestLoansAreReadableThroughTheGateway(t *testing.T) {
	const listBody = `[{"id":1,"bookId":7,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z"}]`
	const notFoundBody = `{"error":"loan not found"}`

	var methods, paths []string
	loans := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/loans/999" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, notFoundBody)
			return
		}
		_, _ = io.WriteString(w, listBody)
	}))
	defer loans.Close()

	gateway := newRouterWithLoans("http://catalog.invalid", loans.URL)

	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != listBody {
		t.Fatalf("body %q, want %q", got, listBody)
	}

	rec = httptest.NewRecorder()
	gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans/1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}

	rec = httptest.NewRecorder()
	gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans/999", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != notFoundBody {
		t.Fatalf("body %q, want %q", got, notFoundBody)
	}

	wantPaths := []string{"/loans", "/loans/1", "/loans/999"}
	if len(paths) != len(wantPaths) {
		t.Fatalf("loans service received paths %v, want %v", paths, wantPaths)
	}
	for i, want := range wantPaths {
		if paths[i] != want {
			t.Fatalf("loans service received paths %v, want %v", paths, wantPaths)
		}
	}
	for _, m := range methods {
		if m != http.MethodGet {
			t.Fatalf("loans service received methods %v, want GET for each", methods)
		}
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	rec = httptest.NewRecorder()
	newRouterWithLoans("http://catalog.invalid", closedURL).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans", nil))

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
