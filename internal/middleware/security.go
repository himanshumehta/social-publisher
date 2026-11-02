package middleware

import (
	"net/http"
	"strings"
)

// MaxRequestBodyBytes caps JSON request bodies. The admin video path uploads
// straight to S3 with presigned URLs, so 1 MiB is far above any legitimate
// JSON payload. The one exception is the API-key ingest route, which the
// router gives its own, larger cap.
const MaxRequestBodyBytes = 1 << 20

// LimitBody rejects request bodies larger than limit bytes; reads past the
// limit fail and the handler's JSON decode returns an error.
func LimitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets response headers for a JSON-only API: no MIME
// sniffing, no framing, no referrer, a CSP that forbids loading anything, and
// no caching of API responses. hsts adds Strict-Transport-Security (enable
// when the service is only reachable over HTTPS).
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("Cross-Origin-Resource-Policy", "same-site")
			if strings.HasPrefix(r.URL.Path, "/api/") {
				h.Set("Cache-Control", "no-store")
			}
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
