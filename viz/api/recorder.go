package api

import (
	"net/http"
	"strings"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   strings.Builder
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	r.body.Write(p)
	return r.ResponseWriter.Write(p)
}
