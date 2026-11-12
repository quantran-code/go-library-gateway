// Command gateway is the address the browser uses for the library API.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("gateway listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, NewRouter()))
}

// defaultCatalogURL is where the catalog service listens when CATALOG_URL is
// not set.
const defaultCatalogURL = "http://localhost:8081"

// defaultLoansURL is where the loans service listens when LOANS_URL is not
// set.
const defaultLoansURL = "http://localhost:8082"

// NewRouter wires the routes the gateway answers itself. Everything else is
// forwarded to the service that owns the resource.
func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	catalogTarget, err := url.Parse(catalogURL())
	if err != nil {
		log.Printf("invalid CATALOG_URL %q, using %s: %v", catalogURL(), defaultCatalogURL, err)
		catalogTarget, _ = url.Parse(defaultCatalogURL)
	}
	catalogProxy := newServiceProxy(catalogTarget)
	r.Get("/api/books", catalogProxy.ServeHTTP)
	r.Get("/api/books/{id}", catalogProxy.ServeHTTP)

	loansTarget, err := url.Parse(loansURL())
	if err != nil {
		log.Printf("invalid LOANS_URL %q, using %s: %v", loansURL(), defaultLoansURL, err)
		loansTarget, _ = url.Parse(defaultLoansURL)
	}
	loansProxy := newServiceProxy(loansTarget)
	r.Post("/api/loans", loansProxy.ServeHTTP)
	r.Get("/api/loans", loansProxy.ServeHTTP)
	r.Get("/api/loans/{id}", loansProxy.ServeHTTP)
	r.Post("/api/loans/{id}/return", loansProxy.ServeHTTP)
	return withRequestID(r)
}

// catalogURL reports the base URL of the catalog service.
func catalogURL() string {
	if u := os.Getenv("CATALOG_URL"); u != "" {
		return u
	}
	return defaultCatalogURL
}

// loansURL reports the base URL of the loans service.
func loansURL() string {
	if u := os.Getenv("LOANS_URL"); u != "" {
		return u
	}
	return defaultLoansURL
}

// newServiceProxy forwards to a service, stripping the /api prefix so
// /api/books reaches the catalog as /books, /api/books/7 as /books/7 and
// /api/loans reaches the loans service as /loans.
func newServiceProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api")
		req.URL.RawPath = ""
		req.Header.Set(requestIDHeader, requestIDFrom(req.Context()))
	}
	return proxy
}
