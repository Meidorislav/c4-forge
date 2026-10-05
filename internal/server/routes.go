package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Meidorislav/c4-forge/internal/buildinfo"
)

// NewHandler returns the root HTTP handler with all routes and middleware.
func NewHandler(log *slog.Logger, opts Options) http.Handler {
	r := chi.NewRouter()
	r.Use(requestID, requestLogger(log), recoverer(log), securityHeaders)

	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(log, opts.ReadinessChecks))

	// /api is reserved for the API: unknown paths there are JSON errors, not
	// the UI's index page.
	r.Route("/api", func(r chi.Router) {
		r.NotFound(apiNotFound)
	})

	r.Handle("/*", uiHandler(opts.UI))

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

func apiNotFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status line is already sent; an encoding error cannot be reported
	// to the client anymore.
	_ = json.NewEncoder(w).Encode(body)
}
