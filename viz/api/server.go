package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/example/cf-edgescout/store"
)

// Server exposes a data API backed by a store.Store.
type Server struct {
	Store          store.Store
	CacheTTL       time.Duration
	DefaultFilters FilterOptions
	AllowedOrigins []string

	now   func() time.Time
	cache *responseCache
}

// Handler constructs the HTTP handler tree, wiring middleware and caching.
func (s *Server) Handler() http.Handler {
	if s.Store == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "store is not configured", http.StatusInternalServerError)
		})
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.CacheTTL > 0 && s.cache == nil {
		s.cache = &responseCache{ttl: s.CacheTTL}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.wrap(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	mux.HandleFunc("/results", s.wrap(s.handleResults))
	mux.HandleFunc("/results/summary", s.wrap(s.handleSummary))
	mux.HandleFunc("/results/timeseries", s.wrap(s.handleTimeseries))
	mux.HandleFunc("/results/", s.wrap(s.handleSource))

	return s.withCORS(mux)
}

func (s *Server) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || s.CacheTTL <= 0 {
			next(w, r)
			return
		}

		key := r.URL.Path + "?" + r.URL.RawQuery
		if entry, ok := s.cache.get(key, s.now()); ok {
			copyHeaders(w.Header(), entry.header)
			w.WriteHeader(entry.status)
			_, _ = w.Write(entry.data)
			return
		}

		recorder := newResponseRecorder(w)
		next(recorder, r)
		if recorder.status >= 200 && recorder.status < 400 {
			s.cache.set(key, cacheEntry{
				data:      []byte(recorder.body.String()),
				status:    recorder.status,
				header:    recorder.Header().Clone(),
				expiresAt: s.now().Add(s.CacheTTL),
			})
		}
	}
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowOrigin(origin, s.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			} else if allowOrigin("*", s.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func allowOrigin(origin string, allowed []string) bool {
	if len(allowed) == 0 {
		return origin == "*"
	}
	for _, candidate := range allowed {
		if candidate == "*" || strings.EqualFold(candidate, origin) {
			return true
		}
	}
	return false
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
