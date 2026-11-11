package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
