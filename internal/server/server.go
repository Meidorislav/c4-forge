// Package server wires the HTTP server: routes, middleware and lifecycle.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server is the c4-forge HTTP server.
type Server struct {
	http            *http.Server
	shutdownTimeout time.Duration
}

// New creates a server that logs to log and, once asked to stop, waits up to
// shutdownTimeout for in-flight requests. checks are reported by /readyz.
func New(log *slog.Logger, shutdownTimeout time.Duration, checks ...ReadinessCheck) *Server {
	return &Server{
		http: &http.Server{
			Handler:           NewHandler(log, checks...),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
			// No global Read/WriteTimeout: the realtime endpoint (ADR-0002)
			// keeps connections open indefinitely. Timeouts for regular
			// endpoints will be applied per route.
			ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn),
		},
		shutdownTimeout: shutdownTimeout,
	}
}

// Serve accepts connections on ln until ctx is cancelled, then shuts down
// gracefully. It returns nil after a clean shutdown.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	errc := make(chan error, 1)
	go func() { errc <- s.http.Serve(ln) }()

	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
