package api

import (
	"encoding/json"
	"net/http"

	"github.com/example/cf-edgescout/store"
)

// Server exposes a minimal API for accessing stored scan results.
type Server struct {
	Store store.Store
}

// Handler returns an http.Handler with routes served by the API server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/results", func(w http.ResponseWriter, r *http.Request) {
		records, err := s.Store.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(records)
	})
	return mux
}
