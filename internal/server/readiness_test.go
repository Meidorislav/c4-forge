package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func getReadyz(t *testing.T, h http.Handler) (int, readyResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil))
	var body readyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body %q: %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

func ok(context.Context) error { return nil }

func TestReadyzAllChecksPass(t *testing.T) {
	h := NewHandler(slog.New(slog.DiscardHandler),
		ReadinessCheck{Name: "database", Check: ok},
		ReadinessCheck{Name: "other", Check: ok},
	)
	code, body := getReadyz(t, h)
	if code != http.StatusOK || body.Status != "ready" {
		t.Errorf("got %d %q, want 200 ready", code, body.Status)
	}
	if body.Checks["database"] != "ok" || body.Checks["other"] != "ok" {
		t.Errorf("checks = %v, want both ok", body.Checks)
	}
}

func TestReadyzWithoutChecksIsReady(t *testing.T) {
	code, body := getReadyz(t, NewHandler(slog.New(slog.DiscardHandler)))
	if code != http.StatusOK || body.Status != "ready" {
		t.Errorf("got %d %q, want 200 ready", code, body.Status)
	}
}

func TestReadyzFailingCheck(t *testing.T) {
	var logs bytes.Buffer
	failing := func(context.Context) error {
		return errors.New("dial tcp 10.0.0.5:5432: connection refused")
	}
	h := NewHandler(testLogger(&logs),
		ReadinessCheck{Name: "database", Check: failing},
		ReadinessCheck{Name: "other", Check: ok},
	)

	code, body := getReadyz(t, h)
	if code != http.StatusServiceUnavailable || body.Status != "not ready" {
		t.Fatalf("got %d %q, want 503 not ready", code, body.Status)
	}
	if body.Checks["database"] != "unavailable" || body.Checks["other"] != "ok" {
		t.Errorf("checks = %v", body.Checks)
	}

	// The error stays in the log and out of the response.
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "10.0.0.5") {
		t.Errorf("response leaks the error: %s", raw)
	}
	if !strings.Contains(logs.String(), "readiness check failed") || !strings.Contains(logs.String(), "10.0.0.5") {
		t.Errorf("failure was not logged with its error: %s", logs.String())
	}
}

func TestReadyzCheckTimesOut(t *testing.T) {
	hanging := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	h := NewHandler(slog.New(slog.DiscardHandler), ReadinessCheck{Name: "database", Check: hanging})

	start := time.Now()
	code, _ := getReadyz(t, h)
	if code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", code)
	}
	if elapsed := time.Since(start); elapsed > readinessTimeout+time.Second {
		t.Errorf("probe took %s, want it bounded by %s", elapsed, readinessTimeout)
	}
}

func TestHealthzIgnoresFailingChecks(t *testing.T) {
	failing := func(context.Context) error { return errors.New("down") }
	h := NewHandler(slog.New(slog.DiscardHandler), ReadinessCheck{Name: "database", Check: failing})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200 even when a dependency is down", rec.Code)
	}
}
