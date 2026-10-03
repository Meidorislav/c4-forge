package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// readinessTimeout bounds each check, so a hanging dependency makes the
// service "not ready" instead of hanging the probe.
const readinessTimeout = 2 * time.Second

// ReadinessCheck is a named dependency that must be available for the
// service to accept traffic.
type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

// readyz reports whether every dependency is available: 200 if so, 503
// otherwise. The response names failing checks but never includes their
// errors, which may reveal internal addresses; those go to the log.
func readyz(log *slog.Logger, checks []ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := make(map[string]string, len(checks))
		ready := true
		for _, c := range checks {
			ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
			err := c.Check(ctx)
			cancel()
			if err != nil {
				ready = false
				results[c.Name] = "unavailable"
				log.WarnContext(r.Context(), "readiness check failed",
					"request_id", requestIDFrom(r.Context()),
					"check", c.Name,
					"error", err,
				)
				continue
			}
			results[c.Name] = "ok"
		}

		status, code := "ready", http.StatusOK
		if !ready {
			status, code = "not ready", http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]any{"status": status, "checks": results})
	}
}
