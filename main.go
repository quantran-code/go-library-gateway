// Command gateway is the address the browser uses for the library API.
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("gateway listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, NewRouter()))
}

// NewRouter wires the routes the gateway answers itself. Everything else is
// forwarded to the service that owns the resource.
func NewRouter() http.Handler {
	return newRouter(catalogURLFromEnv())
}
