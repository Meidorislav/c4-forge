package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Meidorislav/c4-forge/internal/db/dbtest"
)

func noEnv(string) string { return "" }

// syncBuffer is a bytes.Buffer that can be written by the service while the
// test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForLog waits until the logs contain msg.
func waitForLog(t *testing.T, logs *syncBuffer, msg string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(logs.String(), msg) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for log %q; logs:\n%s", msg, logs.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(t.Context(), []string{"--version"}, noEnv, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr: %s", code, ExitOK, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "c4forge ") {
		t.Errorf("stdout = %q, want a version line", stdout.String())
	}
}

func TestUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(t.Context(), []string{"--nope"}, noEnv, &stdout, &stderr); code != ExitConfig {
		t.Errorf("exit code = %d, want %d", code, ExitConfig)
	}
}

func TestInvalidConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	getenv := func(key string) string {
		if key == "C4FORGE_LOG_FORMAT" {
			return "xml"
		}
		return ""
	}
	code := Run(t.Context(), nil, getenv, &stdout, &stderr)
	if code != ExitConfig {
		t.Fatalf("exit code = %d, want %d", code, ExitConfig)
	}
	if !strings.Contains(stderr.String(), "C4FORGE_LOG_FORMAT") {
		t.Errorf("stderr = %q, want it to name the invalid variable", stderr.String())
	}
}

func TestRunUntilCancelled(t *testing.T) {
	databaseURL := dbtest.NewDatabase(t)
	getenv := func(key string) string {
		switch key {
		case "C4FORGE_HTTP_ADDR":
			return "127.0.0.1:0"
		case "C4FORGE_DATABASE_URL":
			return databaseURL
		}
		return ""
	}
	ctx, cancel := context.WithCancel(t.Context())
	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() { done <- Run(ctx, nil, getenv, &stdout, &stderr) }()

	// Wait until startup (database, migrations, listener) is complete.
	waitForLog(t, &stdout, "c4forge started")

	// The service reports itself ready, with the database reachable.
	status, body := get(t, "http://"+listenAddr(t, &stdout)+"/readyz")
	if status != http.StatusOK || !strings.Contains(body, `"database":"ok"`) {
		t.Errorf("/readyz = %d %s, want 200 with the database ok", status, body)
	}
	cancel()

	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d; output: %s%s", code, ExitOK, stdout.String(), stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
	for _, msg := range []string{"database schema is up to date", "c4forge started", "c4forge stopped"} {
		if !strings.Contains(stdout.String(), msg) {
			t.Errorf("logs do not contain %q: %s", msg, stdout.String())
		}
	}

	// Startup migrated the database.
	conn, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	var trgm bool
	if err := conn.QueryRow(t.Context(),
		"SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')").Scan(&trgm); err != nil {
		t.Fatal(err)
	}
	if !trgm {
		t.Error("startup did not apply migrations")
	}
}

// listenAddr returns the address from the "c4forge started" log record.
func listenAddr(t *testing.T, logs *syncBuffer) string {
	t.Helper()
	for line := range strings.Lines(logs.String()) {
		var rec struct {
			Msg  string `json:"msg"`
			Addr string `json:"addr"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Msg == "c4forge started" {
			return rec.Addr
		}
	}
	t.Fatalf("no start record in logs:\n%s", logs.String())
	return ""
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}
