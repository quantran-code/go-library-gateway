package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// requestIDPattern matches the 32 lowercase hex characters a minted request id
// is made of.
var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// nonEmptyLines splits captured log output into the lines that carry text.
func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestHealthReportsTheGatewayIsUp(t *testing.T) {
	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestCatalogueIsForwardedToTheCatalogService(t *testing.T) {
	const stubBody = `[{"id":1,"title":"Dune","author":"Frank Herbert","available":true}]`
	var gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if gotPath != "/books" {
		t.Fatalf("upstream received path %q, want /books", gotPath)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestBookDetailIsForwardedWithTheId(t *testing.T) {
	const stubBody = `{"id":7,"title":"Dune","author":"Frank Herbert","available":true}`
	var gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books/7", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if gotPath != "/books/7" {
		t.Fatalf("upstream received path %q, want /books/7", gotPath)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestUnreachableCatalogAnswers502(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := stub.URL
	stub.Close()
	t.Setenv("CATALOG_URL", closedURL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestBorrowIsForwardedToTheLoansService(t *testing.T) {
	const stubBody = `{"id":1,"bookId":1,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z"}`
	var gotMethod, gotPath, gotBody string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("LOANS_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/loans", strings.NewReader(`{"bookId":1}`)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusCreated)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("upstream received method %q, want POST", gotMethod)
	}
	if gotPath != "/loans" {
		t.Fatalf("upstream received path %q, want /loans", gotPath)
	}
	if gotBody != `{"bookId":1}` {
		t.Fatalf("upstream received body %q, want %q", gotBody, `{"bookId":1}`)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestBorrowErrorStatusesArePassedThrough(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "unknown book", status: http.StatusNotFound, body: `{"error":"book not found"}`},
		{name: "book already on loan", status: http.StatusConflict, body: `{"error":"book already on loan"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer stub.Close()
			t.Setenv("LOANS_URL", stub.URL)

			rec := httptest.NewRecorder()
			NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/loans", strings.NewReader(`{"bookId":1}`)))

			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			if rec.Body.String() != tc.body {
				t.Fatalf("body %q, want %q", rec.Body.String(), tc.body)
			}
		})
	}
}

func TestUnreachableLoansAnswers502(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := stub.URL
	stub.Close()
	t.Setenv("LOANS_URL", closedURL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/loans", strings.NewReader(`{"bookId":1}`)))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestLoansListIsForwardedToTheLoansService(t *testing.T) {
	const stubBody = `[{"id":2,"bookId":1,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z"},{"id":1,"bookId":2,"borrowedAt":"2024-04-01T10:00:00Z","dueAt":"2024-04-15T10:00:00Z"}]`
	var gotMethod, gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("LOANS_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("upstream received method %q, want GET", gotMethod)
	}
	if gotPath != "/loans" {
		t.Fatalf("upstream received path %q, want /loans", gotPath)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestLoanDetailIsForwardedWithTheId(t *testing.T) {
	const stubBody = `{"id":7,"bookId":1,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z"}`
	var gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("LOANS_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans/7", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if gotPath != "/loans/7" {
		t.Fatalf("upstream received path %q, want /loans/7", gotPath)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestReturnIsForwardedToTheLoansService(t *testing.T) {
	const stubBody = `{"id":7,"bookId":1,"borrowedAt":"2024-05-01T10:00:00Z","dueAt":"2024-05-15T10:00:00Z","returnedAt":"2024-05-10T09:30:00Z"}`
	var gotMethod, gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("LOANS_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/loans/7/return", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("upstream received method %q, want POST", gotMethod)
	}
	if gotPath != "/loans/7/return" {
		t.Fatalf("upstream received path %q, want /loans/7/return", gotPath)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestLoanErrorStatusesArePassedThrough(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		status int
		body   string
	}{
		{name: "invalid loan id", method: http.MethodPost, path: "/api/loans/abc/return", status: http.StatusBadRequest, body: `{"error":"invalid loan id"}`},
		{name: "unknown loan", method: http.MethodGet, path: "/api/loans/999", status: http.StatusNotFound, body: `{"error":"loan not found"}`},
		{name: "loan already returned", method: http.MethodPost, path: "/api/loans/7/return", status: http.StatusConflict, body: `{"error":"loan already returned"}`},
		{name: "catalog unavailable", method: http.MethodPost, path: "/api/loans/7/return", status: http.StatusBadGateway, body: `{"error":"catalog unavailable"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer stub.Close()
			t.Setenv("LOANS_URL", stub.URL)

			rec := httptest.NewRecorder()
			NewRouter().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			if rec.Body.String() != tc.body {
				t.Fatalf("body %q, want %q", rec.Body.String(), tc.body)
			}
			if want := strings.TrimPrefix(tc.path, "/api"); gotPath != want {
				t.Fatalf("upstream received path %q, want %q", gotPath, want)
			}
		})
	}
}

func TestLoansNonNumericIdIsForwardedUnchanged(t *testing.T) {
	var gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid loan id"}`))
	}))
	defer stub.Close()
	t.Setenv("LOANS_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans/abc", nil))

	if gotPath != "/loans/abc" {
		t.Fatalf("upstream received path %q, want /loans/abc", gotPath)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRequestIDIsPreservedForwardedAndEchoed(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{name: "32 hex characters", id: "9f2c4a1e7b3d4086a5c2e1f0b7d39a64"},
		{name: "a value the caller made up", id: "client-abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotHeader string
			stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Get("X-Request-Id")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			defer stub.Close()
			t.Setenv("CATALOG_URL", stub.URL)

			req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
			req.Header.Set("X-Request-Id", tc.id)
			rec := httptest.NewRecorder()
			NewRouter().ServeHTTP(rec, req)

			if gotHeader != tc.id {
				t.Fatalf("upstream received X-Request-Id %q, want %q", gotHeader, tc.id)
			}
			if got := rec.Header().Get("X-Request-Id"); got != tc.id {
				t.Fatalf("response X-Request-Id %q, want %q", got, tc.id)
			}
		})
	}
}

func TestRequestIDIsMintedWhenTheCallerSendsNone(t *testing.T) {
	var gotHeader string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Request-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books", nil))

	if !requestIDPattern.MatchString(gotHeader) {
		t.Fatalf("upstream received X-Request-Id %q, want 32 lowercase hex characters", gotHeader)
	}
	if got := rec.Header().Get("X-Request-Id"); got != gotHeader {
		t.Fatalf("response X-Request-Id %q, want the forwarded %q", got, gotHeader)
	}
}

func TestGatewayGenerated404CarriesTheRequestID(t *testing.T) {
	const id = "9f2c4a1e7b3d4086a5c2e1f0b7d39a64"

	t.Run("caller supplied id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
		req.Header.Set("X-Request-Id", id)
		rec := httptest.NewRecorder()
		NewRouter().ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d, want %d", rec.Code, http.StatusNotFound)
		}
		if got := rec.Header().Get("X-Request-Id"); got != id {
			t.Fatalf("response X-Request-Id %q, want %q", got, id)
		}
	})

	t.Run("no inbound id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/unknown", nil))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d, want %d", rec.Code, http.StatusNotFound)
		}
		if got := rec.Header().Get("X-Request-Id"); !requestIDPattern.MatchString(got) {
			t.Fatalf("response X-Request-Id %q, want 32 lowercase hex characters", got)
		}
	})
}

func TestGatewayGenerated502CarriesTheRequestID(t *testing.T) {
	const id = "9f2c4a1e7b3d4086a5c2e1f0b7d39a64"
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := stub.URL
	stub.Close()
	t.Setenv("CATALOG_URL", closedURL)

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.Header.Set("X-Request-Id", id)
	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if got := rec.Header().Get("X-Request-Id"); got != id {
		t.Fatalf("response X-Request-Id %q, want %q", got, id)
	}
}

func TestEachRequestLogsOneLineWithMethodPathStatusAndRequestID(t *testing.T) {
	const id = "9f2c4a1e7b3d4086a5c2e1f0b7d39a64"
	var gotHeader string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Request-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(previous)

	req := httptest.NewRequest(http.MethodGet, "/api/books", nil)
	req.Header.Set("X-Request-Id", id)
	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, req)

	lines := nonEmptyLines(buf.String())
	if len(lines) != 1 {
		t.Fatalf("logged %d lines, want 1: %q", len(lines), buf.String())
	}
	for _, want := range []string{"method=GET", "path=/api/books", "status=200", "request_id=" + id} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("log line %q does not contain %q", lines[0], want)
		}
	}
	if gotHeader != id {
		t.Fatalf("upstream received X-Request-Id %q, want %q", gotHeader, id)
	}
	if got := rec.Header().Get("X-Request-Id"); got != id {
		t.Fatalf("response X-Request-Id %q, want %q", got, id)
	}
}
