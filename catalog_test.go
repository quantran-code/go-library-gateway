package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBooksAreForwardedToCatalog(t *testing.T) {
	const body = `[{"id":1,"title":"The Go Programming Language","author":"Donovan","available":true}]`

	var paths []string
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer catalog.Close()

	gateway := newRouter(catalog.URL)

	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body %q, want %q", got, body)
	}
	if len(paths) != 1 || paths[0] != "/books" {
		t.Fatalf("catalog received paths %v, want [/books]", paths)
	}

	gateway.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/books/42", nil))

	if len(paths) != 2 || paths[1] != "/books/42" {
		t.Fatalf("catalog received paths %v, want /books/42 last", paths)
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "/api") {
			t.Fatalf("catalog received path %q, the /api prefix was not stripped", p)
		}
	}
}

func TestCatalogErrorsPassThrough(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "unknown book", status: http.StatusNotFound, body: `{"error":"book not found"}`},
		{name: "invalid id", status: http.StatusBadRequest, body: `{"error":"invalid book id"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer catalog.Close()

			rec := httptest.NewRecorder()
			newRouter(catalog.URL).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/books/999", nil))

			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tc.body {
				t.Fatalf("body %q, want %q", got, tc.body)
			}
		})
	}
}

func TestCatalogURLFromEnv(t *testing.T) {
	t.Setenv("CATALOG_URL", "")
	if got := catalogURLFromEnv(); got != "http://localhost:8081" {
		t.Fatalf("empty CATALOG_URL resolved to %q, want http://localhost:8081", got)
	}

	t.Setenv("CATALOG_URL", "http://catalog.internal:9000")
	if got := catalogURLFromEnv(); got != "http://catalog.internal:9000" {
		t.Fatalf("CATALOG_URL resolved to %q, want http://catalog.internal:9000", got)
	}
}
