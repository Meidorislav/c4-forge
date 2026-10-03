package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Meidorislav/c4-forge/internal/buildinfo"
)

// NewHandler returns the root HTTP handler with all routes and middleware.
// checks are the dependencies behind /readyz.
func NewHandler(log *slog.Logger, checks ...ReadinessCheck) http.Handler {
	r := chi.NewRouter()
	r.Use(requestID, requestLogger(log), recoverer(log))

	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(log, checks))

	return r
}

// healthz reports that the process is alive. It does not check
// dependencies; see readyz for that.
func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": buildinfo.Version,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status line is already sent; an encoding error cannot be reported
	// to the client anymore.
	_ = json.NewEncoder(w).Encode(body)
}
