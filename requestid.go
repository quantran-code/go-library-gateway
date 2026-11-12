package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
)

// requestIDHeader carries the id minted once per user action in the browser.
const requestIDHeader = "X-Request-Id"

// requestIDKey is the request context key holding the request id.
type requestIDKey struct{}

// newRequestID returns 32 lowercase hex characters: 16 bytes from crypto/rand,
// hex-encoded.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		log.Printf("generating request id: %v", err)
		return ""
	}
	return hex.EncodeToString(b[:])
}

// requestIDFrom reports the request id withRequestID stored for this request. It
// is empty when the context holds none.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// withRequestID keeps an incoming X-Request-Id unchanged, mints one when the
// caller sent none, stores it in the request context, echoes it on the response
// before the handler runs — so the router's own 404 and the proxy's 502 carry it
// too — and logs one line per request.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		w.Header().Set(requestIDHeader, id)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))
		log.Printf("gateway method=%s path=%s status=%d request_id=%s",
			r.Method, r.URL.Path, rec.status, id)
	})
}

// statusRecorder captures the status code the handler writes so the middleware
// can log it. Write without an explicit WriteHeader records 200.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}
