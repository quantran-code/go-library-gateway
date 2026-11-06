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

// catalogURLFromEnv reports the catalog service's address.
func catalogURLFromEnv() string {
	if url := os.Getenv("CATALOG_URL"); url != "" {
		return url
	}
	return defaultCatalogURL
}

// newRouter wires the routes the gateway answers itself and the catalogue
// routes it forwards to the catalog service.
func newRouter(catalogURL string) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	r.Get("/api/books", catalogProxy(catalogURL))
	r.Get("/api/books/{id}", catalogProxy(catalogURL))
	return r
}

// catalogProxy forwards a request to the catalog service with the /api prefix
// the browser uses stripped off, so /api/books becomes /books. The upstream
// status code and body are copied through unchanged, which is what carries the
// catalog's 400 and 404 bodies to the browser verbatim.
func catalogProxy(catalogURL string) http.HandlerFunc {
	client := &http.Client{Timeout: 10 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		upstream := catalogURL + strings.TrimPrefix(r.URL.Path, "/api")

		req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, nil)
		if err != nil {
			log.Printf("building catalog request for %s: %v", upstream, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "catalog unavailable"})
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("calling catalog at %s: %v", upstream, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "catalog unavailable"})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		w.Header().Set("Content-Type", "application/json")
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
