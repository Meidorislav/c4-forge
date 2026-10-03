package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func noEnv(string) string { return "" }

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
	getenv := func(key string) string {
		switch key {
		case "C4FORGE_HTTP_ADDR":
			return "127.0.0.1:0"
		case "C4FORGE_DATABASE_URL":
			return "postgres://c4forge:c4forge@localhost:5432/c4forge"
		}
		return ""
	}
	ctx, cancel := context.WithCancel(t.Context())
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- Run(ctx, nil, getenv, &stdout, &stderr) }()

	// Let the server start before stopping it.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d; output: %s%s", code, ExitOK, stdout.String(), stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
	for _, msg := range []string{"c4forge started", "c4forge stopped"} {
		if !strings.Contains(stdout.String(), msg) {
			t.Errorf("logs do not contain %q: %s", msg, stdout.String())
		}
	}
}
