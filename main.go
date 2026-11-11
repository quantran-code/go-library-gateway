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

// NewRouter wires the routes the gateway answers itself. Everything else is
// forwarded to the service that owns the resource.
func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	target, err := url.Parse(catalogURL())
	if err != nil {
		log.Printf("invalid CATALOG_URL %q, using %s: %v", catalogURL(), defaultCatalogURL, err)
		target, _ = url.Parse(defaultCatalogURL)
	}
	proxy := newCatalogProxy(target)
	r.Get("/api/books", proxy.ServeHTTP)
	r.Get("/api/books/{id}", proxy.ServeHTTP)
	return r
}

// catalogURL reports the base URL of the catalog service.
func catalogURL() string {
	if u := os.Getenv("CATALOG_URL"); u != "" {
		return u
	}
	return defaultCatalogURL
}

// newCatalogProxy forwards to the catalog service, stripping the /api prefix
// so /api/books reaches the service as /books and /api/books/7 as /books/7.
func newCatalogProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api")
		req.URL.RawPath = ""
	}
	return proxy
}
