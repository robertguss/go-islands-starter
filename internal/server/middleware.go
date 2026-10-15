package server

import (
	"crypto/rand"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

func Headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		recorded := &responseStatus{ResponseWriter: w, status: 200}
		next.ServeHTTP(recorded, r)
		// The matched route has no query values, OIDC codes, record content, or email.
		slog.Info("http_request", "route", r.Pattern, "status", recorded.status, "duration_ms", time.Since(start).Milliseconds(), "request_id", id)
	})
}

type responseStatus struct {
	http.ResponseWriter
	status int
}

func (w *responseStatus) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseStatus) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type limitEntry struct {
	count int
	until time.Time
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]limitEntry
	max     int
}

func newLimiter(max int) *limiter { return &limiter{entries: map[string]limitEntry{}, max: max} }
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, e := range l.entries {
		if !now.Before(e.until) {
			delete(l.entries, k)
		}
	}
	e := l.entries[key]
	if e.count == 0 {
		if len(l.entries) >= 1024 {
			return false
		}
		e.until = now.Add(time.Minute)
	}
	if e.count >= l.max {
		return false
	}
	e.count++
	l.entries[key] = e
	return true
}
func (l *limiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Forwarded addresses are deliberately untrusted; see the proxy setup runbook.
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !l.allow(host, time.Now()) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Please wait a minute before trying again.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
