package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testLogger returns a JSON logger writing to buf, including debug records.
func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// logRecords decodes the JSON log lines written to buf.
func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("invalid log line %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func TestHealthz(t *testing.T) {
	h := NewHandler(slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body["status"] != "ok" || body["version"] == "" {
		t.Errorf("body = %v, want status ok and a version", body)
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	h := NewHandler(slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		reused   bool
	}{
		{"generated when missing", "", false},
		{"reused when valid", "abc-123", true},
		{"replaced when invalid", "bad id\nwith newline", false},
		{"replaced when too long", strings.Repeat("a", 65), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := NewHandler(testLogger(&logs))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
			if tt.incoming != "" {
				req.Header.Set(requestIDHeader, tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			id := rec.Header().Get(requestIDHeader)
			if id == "" {
				t.Fatal("response has no request ID")
			}
			if tt.reused != (id == tt.incoming) {
				t.Errorf("request ID = %q, incoming %q, want reused=%v", id, tt.incoming, tt.reused)
			}
			recs := logRecords(t, &logs)
			if len(recs) != 1 || recs[0]["request_id"] != id {
				t.Errorf("log records = %v, want one record with request_id %q", recs, id)
			}
		})
	}
}

func TestPanicBecomes500(t *testing.T) {
	var logs bytes.Buffer
	log := testLogger(&logs)
	boom := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	h := requestID(requestLogger(log)(recoverer(log)(boom)))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	recs := logRecords(t, &logs)
	if len(recs) != 2 {
		t.Fatalf("got %d log records, want 2 (panic + request): %v", len(recs), recs)
	}
	if recs[0]["msg"] != "panic in handler" || recs[0]["panic"] != "boom" || recs[0]["stack"] == "" {
		t.Errorf("panic record = %v", recs[0])
	}
	if recs[1]["status"] != float64(http.StatusInternalServerError) {
		t.Errorf("request record status = %v, want 500", recs[1]["status"])
	}
}

func TestServeAndGracefulShutdown(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(slog.New(slog.DiscardHandler), 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() = %v, want nil after shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return after context cancellation")
	}
}
