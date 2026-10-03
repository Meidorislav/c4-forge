package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testDatabaseURL = "postgres://c4forge:s3cret@localhost:5432/c4forge"

// env returns a getenv function serving vars on top of a valid required set.
func env(vars map[string]string) func(string) string {
	all := map[string]string{"C4FORGE_DATABASE_URL": testDatabaseURL}
	for k, v := range vars {
		all[k] = v
	}
	return func(key string) string { return all[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Default()
	want.DatabaseURL = testDatabaseURL
	if cfg != want {
		t.Errorf("Load() = %+v, want defaults %+v", cfg, want)
	}
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"C4FORGE_HTTP_ADDR":        "127.0.0.1:9090",
		"C4FORGE_LOG_LEVEL":        "debug",
		"C4FORGE_LOG_FORMAT":       "TEXT",
		"C4FORGE_SHUTDOWN_TIMEOUT": "3s",
		"C4FORGE_DATABASE_URL":     "host=db user=c4forge dbname=c4forge pool_max_conns=20",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		HTTPAddr:        "127.0.0.1:9090",
		LogLevel:        slog.LevelDebug,
		LogFormat:       LogFormatText,
		ShutdownTimeout: 3 * time.Second,
		DatabaseURL:     "host=db user=c4forge dbname=c4forge pool_max_conns=20",
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
		{"database url not a dsn", "C4FORGE_DATABASE_URL", "definitely not a dsn"},
		{"database url bad port", "C4FORGE_DATABASE_URL", "postgres://u:p@localhost:notaport/db"},
		{"database url bad pool setting", "C4FORGE_DATABASE_URL", "postgres://u:p@localhost/db?pool_max_conns=abc"},
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

func TestLoadRequiresDatabaseURL(t *testing.T) {
	_, err := Load(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "C4FORGE_DATABASE_URL: required") {
		t.Fatalf("Load() error = %v, want a missing C4FORGE_DATABASE_URL error", err)
	}
}

func TestLoadDoesNotLeakDatabasePassword(t *testing.T) {
	for _, url := range []string{
		"postgres://c4forge:s3cret@localhost:5432/db?sslmode=bogus",
		"host=localhost password=s3cret sslmode=bogus",
	} {
		_, err := Load(env(map[string]string{"C4FORGE_DATABASE_URL": url}))
		if err == nil {
			t.Fatalf("Load() with %q: expected an error", url)
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("error leaks the password: %v", err)
		}
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
