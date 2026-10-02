package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg != Default() {
		t.Errorf("Load() = %+v, want defaults %+v", cfg, Default())
	}
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"C4FORGE_HTTP_ADDR":        "127.0.0.1:9090",
		"C4FORGE_LOG_LEVEL":        "debug",
		"C4FORGE_LOG_FORMAT":       "TEXT",
		"C4FORGE_SHUTDOWN_TIMEOUT": "3s",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		HTTPAddr:        "127.0.0.1:9090",
		LogLevel:        slog.LevelDebug,
		LogFormat:       LogFormatText,
		ShutdownTimeout: 3 * time.Second,
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"addr without port", "C4FORGE_HTTP_ADDR", "localhost"},
		{"unknown log level", "C4FORGE_LOG_LEVEL", "verbose"},
		{"unknown log format", "C4FORGE_LOG_FORMAT", "xml"},
		{"timeout without unit", "C4FORGE_SHUTDOWN_TIMEOUT", "10"},
		{"negative timeout", "C4FORGE_SHUTDOWN_TIMEOUT", "-1s"},
		{"zero timeout", "C4FORGE_SHUTDOWN_TIMEOUT", "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(map[string]string{tt.key: tt.value}))
			if err == nil {
				t.Fatalf("Load() with %s=%q: expected an error", tt.key, tt.value)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error %q does not name the variable %s", err, tt.key)
			}
		})
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := Load(env(map[string]string{
		"C4FORGE_HTTP_ADDR":  "nope",
		"C4FORGE_LOG_FORMAT": "xml",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"C4FORGE_HTTP_ADDR", "C4FORGE_LOG_FORMAT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}
