package app

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeAddr(t *testing.T) {
	tests := map[string]string{
		":8080":           "127.0.0.1:8080",
		"0.0.0.0:8080":    "127.0.0.1:8080",
		"[::]:8080":       "127.0.0.1:8080",
		"127.0.0.1:18090": "127.0.0.1:18090",
		"localhost:9000":  "localhost:9000",
		"[::1]:9000":      "[::1]:9000",
		"10.0.0.7:8080":   "10.0.0.7:8080",
	}
	for listen, want := range tests {
		if got := probeAddr(listen); got != want {
			t.Errorf("probeAddr(%q) = %q, want %q", listen, got, want)
		}
	}
}

// runHealthcheck runs "c4forge healthcheck" against a server with the given /readyz handler.
func runHealthcheck(t *testing.T, readyz http.HandlerFunc) (int, string, string) {
	t.Helper()
	srv := httptest.NewServer(readyz)
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")
	return runHealthcheckAt(t, addr)
}

func runHealthcheckAt(t *testing.T, addr string) (int, string, string) {
	t.Helper()
	getenv := func(key string) string {
		switch key {
		case "C4FORGE_HTTP_ADDR":
			return addr
		case "C4FORGE_DATABASE_URL":
			return "postgres://c4forge:c4forge@localhost:5432/c4forge"
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	code := Run(t.Context(), []string{"healthcheck"}, getenv, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestHealthcheckReady(t *testing.T) {
	var path string
	code, stdout, stderr := runHealthcheck(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	if path != "/readyz" {
		t.Errorf("probed %q, want /readyz", path)
	}
	if strings.TrimSpace(stdout) != "ok" {
		t.Errorf("stdout = %q, want ok", stdout)
	}
}

func TestHealthcheckNotReady(t *testing.T) {
	code, _, stderr := runHealthcheck(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"checks":{"database":"unavailable"},"status":"not ready"}`))
	})
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "503") || !strings.Contains(stderr, `"database":"unavailable"`) {
		t.Errorf("stderr = %q, want the status and the failing check", stderr)
	}
}

func TestHealthcheckServerDown(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	code, _, stderr := runHealthcheckAt(t, addr)
	if code != ExitError || stderr == "" {
		t.Errorf("exit code = %d, stderr = %q; want a failure with a reason", code, stderr)
	}
}

func TestHealthcheckTimesOut(t *testing.T) {
	old := healthcheckTimeout
	healthcheckTimeout = 200 * time.Millisecond
	t.Cleanup(func() { healthcheckTimeout = old })

	start := time.Now()
	// The handler hangs until the client gives up and the request is cancelled.
	code, _, _ := runHealthcheck(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("healthcheck took %s, want it bounded by the timeout", elapsed)
	}
}

func TestHealthcheckInvalidConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(t.Context(), []string{"healthcheck"}, func(string) string { return "" }, &stdout, &stderr)
	if code != ExitConfig {
		t.Errorf("exit code = %d, want %d", code, ExitConfig)
	}
}
