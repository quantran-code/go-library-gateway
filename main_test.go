package main

import (
	"bytes"
	"io"
	"log"
	"mime"
	"mime/multipart"
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

func TestBookImportForwardsTheMultipartBodyUnchanged(t *testing.T) {
	const stubBody = `{"imported":1,"skipped":0}`
	var gotMethod, gotPath, gotContentType string
	var gotBody []byte
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "books.csv")
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := part.Write([]byte("title,author,available\r\nDune,Frank Herbert,true\r\n")); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing the multipart writer: %v", err)
	}
	sent := body.Bytes()
	contentType := mw.FormDataContentType()

	req := httptest.NewRequest(http.MethodPost, "/api/books/import", bytes.NewReader(sent))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusCreated)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("upstream received method %q, want POST", gotMethod)
	}
	if gotPath != "/books/import" {
		t.Fatalf("upstream received path %q, want /books/import", gotPath)
	}
	if !bytes.Equal(gotBody, sent) {
		t.Fatalf("upstream received body %q, want the bytes sent %q", gotBody, sent)
	}
	wantMediaType, wantParams, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parsing the sent Content-Type %q: %v", contentType, err)
	}
	gotMediaType, gotParams, err := mime.ParseMediaType(gotContentType)
	if err != nil {
		t.Fatalf("parsing the upstream Content-Type %q: %v", gotContentType, err)
	}
	if gotMediaType != wantMediaType || gotParams["boundary"] != wantParams["boundary"] {
		t.Fatalf("upstream received Content-Type %q, want %q", gotContentType, contentType)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestBooksExportIsForwardedWithItsHeaders(t *testing.T) {
	const (
		stubBody        = "id,title,author,available\r\n1,\"Dune, \"\"Deluxe\"\"\",Frank Herbert,true\r\n"
		stubContentType = "text/csv; charset=utf-8"
		stubDisposition = `attachment; filename="books.csv"`
	)
	var gotMethod, gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", stubContentType)
		w.Header().Set("Content-Disposition", stubDisposition)
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books/export", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("upstream received method %q, want GET", gotMethod)
	}
	if gotPath != "/books/export" {
		t.Fatalf("upstream received path %q, want /books/export", gotPath)
	}
	if got := rec.Header().Get("Content-Type"); got != stubContentType {
		t.Fatalf("Content-Type %q, want %q", got, stubContentType)
	}
	if got := rec.Header().Get("Content-Disposition"); got != stubDisposition {
		t.Fatalf("Content-Disposition %q, want %q", got, stubDisposition)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want the CSV %q", rec.Body.String(), stubBody)
	}
}

func TestLoansExportIsForwardedWithItsHeaders(t *testing.T) {
	const (
		stubBody        = "id,bookId,borrowedAt,dueAt,returnedAt\r\n7,1,2024-05-01T10:00:00Z,2024-05-15T10:00:00Z,\r\n"
		stubContentType = "text/csv; charset=utf-8"
		stubDisposition = `attachment; filename="loans.csv"`
	)
	var gotMethod, gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", stubContentType)
		w.Header().Set("Content-Disposition", stubDisposition)
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("LOANS_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/loans/export", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("upstream received method %q, want GET", gotMethod)
	}
	if gotPath != "/loans/export" {
		t.Fatalf("upstream received path %q, want /loans/export", gotPath)
	}
	if got := rec.Header().Get("Content-Type"); got != stubContentType {
		t.Fatalf("Content-Type %q, want %q", got, stubContentType)
	}
	if got := rec.Header().Get("Content-Disposition"); got != stubDisposition {
		t.Fatalf("Content-Disposition %q, want %q", got, stubDisposition)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want the CSV %q", rec.Body.String(), stubBody)
	}
}

func TestBookImportErrorStatusesArePassedThrough(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "unsupported file type", status: http.StatusUnsupportedMediaType, body: `{"error":"unsupported file type"}`},
		{name: "file too large", status: http.StatusRequestEntityTooLarge, body: `{"error":"file too large"}`},
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
			t.Setenv("CATALOG_URL", stub.URL)

			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			part, err := mw.CreateFormFile("file", "books.txt")
			if err != nil {
				t.Fatalf("creating the file part: %v", err)
			}
			if _, err := part.Write([]byte("title,author,available\r\n")); err != nil {
				t.Fatalf("writing the file part: %v", err)
			}
			if err := mw.Close(); err != nil {
				t.Fatalf("closing the multipart writer: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/books/import", bytes.NewReader(body.Bytes()))
			req.Header.Set("Content-Type", mw.FormDataContentType())
			rec := httptest.NewRecorder()
			NewRouter().ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			if gotPath != "/books/import" {
				t.Fatalf("upstream received path %q, want /books/import", gotPath)
			}
			if rec.Body.String() != tc.body {
				t.Fatalf("body %q, want %q", rec.Body.String(), tc.body)
			}
		})
	}
}

func TestUnreachableCatalogAnswers502ForExport(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := stub.URL
	stub.Close()
	t.Setenv("CATALOG_URL", closedURL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books/export", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if body := rec.Body.String(); body != "" {
		t.Fatalf("body %q, want no file at all", body)
	}
}

func TestImportJobSubmitIsForwardedUnchanged(t *testing.T) {
	const stubBody = `{"jobId":"9f2c4a1e-7b3d-4086-a5c2-e1f0b7d39a64","status":"queued","totalRows":2,"batchSize":100,"batches":1}`
	var gotMethod, gotPath, gotContentType string
	var gotBody []byte
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "books.csv")
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := part.Write([]byte("title,author,available\r\nDune,Frank Herbert,true\r\n")); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing the multipart writer: %v", err)
	}
	sent := body.Bytes()
	contentType := mw.FormDataContentType()

	req := httptest.NewRequest(http.MethodPost, "/api/books/import-jobs", bytes.NewReader(sent))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusAccepted)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("upstream received method %q, want POST", gotMethod)
	}
	if gotPath != "/books/import-jobs" {
		t.Fatalf("upstream received path %q, want /books/import-jobs", gotPath)
	}
	if !bytes.Equal(gotBody, sent) {
		t.Fatalf("upstream received body %q, want the bytes sent %q", gotBody, sent)
	}
	wantMediaType, wantParams, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parsing the sent Content-Type %q: %v", contentType, err)
	}
	gotMediaType, gotParams, err := mime.ParseMediaType(gotContentType)
	if err != nil {
		t.Fatalf("parsing the upstream Content-Type %q: %v", gotContentType, err)
	}
	if gotMediaType != wantMediaType || gotParams["boundary"] != wantParams["boundary"] {
		t.Fatalf("upstream received Content-Type %q, want %q", gotContentType, contentType)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestImportJobStatusIsForwarded(t *testing.T) {
	const (
		jobID    = "9f2c4a1e-7b3d-4086-a5c2-e1f0b7d39a64"
		stubBody = `{"jobId":"9f2c4a1e-7b3d-4086-a5c2-e1f0b7d39a64","status":"processing","totalRows":250,"processedRows":100,"imported":100,"skipped":0,"batches":3,"completedBatches":1,"errors":[],"more":false,"createdAt":"2024-05-01T10:00:00Z","updatedAt":"2024-05-01T10:00:01Z"}`
	)
	var gotMethod, gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stubBody))
	}))
	defer stub.Close()
	t.Setenv("CATALOG_URL", stub.URL)

	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books/import-jobs/"+jobID, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("upstream received method %q, want GET", gotMethod)
	}
	if want := "/books/import-jobs/" + jobID; gotPath != want {
		t.Fatalf("upstream received path %q, want %q", gotPath, want)
	}
	if rec.Body.String() != stubBody {
		t.Fatalf("body %q, want %q", rec.Body.String(), stubBody)
	}
}

func TestImportJobErrorStatusesArePassedThrough(t *testing.T) {
	const jobID = "9f2c4a1e-7b3d-4086-a5c2-e1f0b7d39a64"
	cases := []struct {
		name   string
		method string
		path   string
		status int
		body   string
	}{
		{name: "invalid import file", method: http.MethodPost, path: "/api/books/import-jobs", status: http.StatusBadRequest, body: `{"error":"invalid import file","rows":[{"line":2,"error":"title is required"}],"more":false}`},
		{name: "unknown job", method: http.MethodGet, path: "/api/books/import-jobs/" + jobID, status: http.StatusNotFound, body: `{"error":"job not found"}`},
		{name: "queue unavailable", method: http.MethodPost, path: "/api/books/import-jobs", status: http.StatusServiceUnavailable, body: `{"error":"queue unavailable"}`},
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
			t.Setenv("CATALOG_URL", stub.URL)

			rec := httptest.NewRecorder()
			NewRouter().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			if want := strings.TrimPrefix(tc.path, "/api"); gotPath != want {
				t.Fatalf("upstream received path %q, want %q", gotPath, want)
			}
			if rec.Body.String() != tc.body {
				t.Fatalf("body %q, want %q", rec.Body.String(), tc.body)
			}
		})
	}
}
