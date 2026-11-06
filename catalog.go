package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// defaultCatalogURL is where the catalog service listens during local
// development, used when CATALOG_URL is not set.
const defaultCatalogURL = "http://localhost:8081"

// defaultLoansURL is where the loans service listens during local
// development, used when LOANS_URL is not set.
const defaultLoansURL = "http://localhost:8082"

// requestIDHeader is the header that carries the browser's request id in both
// directions through the gateway. The gateway copies it and never mints one.
const requestIDHeader = "X-Request-Id"

// catalogURLFromEnv reports the catalog service's address.
func catalogURLFromEnv() string {
	if url := os.Getenv("CATALOG_URL"); url != "" {
		return url
	}
	return defaultCatalogURL
}

// loansURLFromEnv reports the loans service's address.
func loansURLFromEnv() string {
	if url := os.Getenv("LOANS_URL"); url != "" {
		return url
	}
	return defaultLoansURL
}

// newRouter wires the routes the gateway answers itself and the catalogue
// routes it forwards to the catalog service, with the loans service at its
// local development address.
func newRouter(catalogURL string) http.Handler {
	return newRouterWithLoans(catalogURL, defaultLoansURL)
}

// newRouterWithLoans wires the routes the gateway answers itself and the
// catalogue and loans routes it forwards to the services that own them.
func newRouterWithLoans(catalogURL string, loansURL string) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	r.Get("/api/books", catalogProxy(catalogURL))
	r.Get("/api/books/{id}", catalogProxy(catalogURL))
	r.Post("/api/loans", proxyTo(loansURL, "loans unavailable"))
	r.Post("/api/loans/{id}/return", proxyTo(loansURL, "loans unavailable"))
	r.Get("/api/loans", proxyTo(loansURL, "loans unavailable"))
	r.Get("/api/loans/{id}", proxyTo(loansURL, "loans unavailable"))
	return r
}

// catalogProxy forwards a request to the catalog service with the /api prefix
// the browser uses stripped off, so /api/books becomes /books. The upstream
// status code and body are copied through unchanged, which is what carries the
// catalog's 400 and 404 bodies to the browser verbatim.
func catalogProxy(catalogURL string) http.HandlerFunc {
	return proxyTo(catalogURL, "catalog unavailable")
}

// proxyTo forwards a request to baseURL with the /api prefix the browser uses
// stripped off, carrying the incoming method and body upstream, so a POST
// reaches the service that owns the resource. The upstream status code and
// body are copied through unchanged. An incoming X-Request-Id is copied onto
// the upstream request and the upstream's X-Request-Id is copied back to the
// browser; the gateway never generates or replaces an id. A transport failure
// answers 502 with {"error": unavailableMessage}.
func proxyTo(baseURL string, unavailableMessage string) http.HandlerFunc {
	client := &http.Client{Timeout: 10 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		upstream := baseURL + strings.TrimPrefix(r.URL.Path, "/api")

		req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, r.Body)
		if err != nil {
			log.Printf("building upstream request for %s: %v", upstream, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": unavailableMessage})
			return
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if requestID := r.Header.Get(requestIDHeader); requestID != "" {
			req.Header.Set(requestIDHeader, requestID)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("calling upstream at %s: %v", upstream, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": unavailableMessage})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		w.Header().Set("Content-Type", "application/json")
		if requestID := resp.Header.Get(requestIDHeader); requestID != "" {
			w.Header().Set(requestIDHeader, requestID)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}
}

// writeJSON writes payload as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
