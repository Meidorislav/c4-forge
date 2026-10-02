// Package config loads the service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

// LogFormat selects how log records are encoded.
type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// Config is the complete service configuration.
type Config struct {
	// HTTPAddr is the address the HTTP server listens on (C4FORGE_HTTP_ADDR).
	HTTPAddr string
	// LogLevel is the minimum level of emitted log records (C4FORGE_LOG_LEVEL).
	LogLevel slog.Level
	// LogFormat is the log encoding (C4FORGE_LOG_FORMAT).
	LogFormat LogFormat
	// ShutdownTimeout bounds how long in-flight requests may take to finish
	// after a shutdown signal (C4FORGE_SHUTDOWN_TIMEOUT).
	ShutdownTimeout time.Duration
}

// Default returns the configuration used when no variables are set.
func Default() Config {
	return Config{
		HTTPAddr:        ":8080",
		LogLevel:        slog.LevelInfo,
		LogFormat:       LogFormatJSON,
		ShutdownTimeout: 10 * time.Second,
	}
}

// Load reads the configuration using getenv (usually os.Getenv). Unset or
// empty variables keep their defaults. All invalid values are reported
// together.
func Load(getenv func(string) string) (Config, error) {
	cfg := Default()
	var errs []error

	if v := getenv("C4FORGE_HTTP_ADDR"); v != "" {
		if _, _, err := net.SplitHostPort(v); err != nil {
			errs = append(errs, fmt.Errorf("C4FORGE_HTTP_ADDR: %q is not a host:port address", v))
		} else {
			cfg.HTTPAddr = v
		}
	}

	if v := getenv("C4FORGE_LOG_LEVEL"); v != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("C4FORGE_LOG_LEVEL: %q is not one of debug, info, warn, error", v))
		} else {
			cfg.LogLevel = level
		}
	}

	if v := getenv("C4FORGE_LOG_FORMAT"); v != "" {
		switch f := LogFormat(strings.ToLower(v)); f {
		case LogFormatJSON, LogFormatText:
			cfg.LogFormat = f
		default:
			errs = append(errs, fmt.Errorf("C4FORGE_LOG_FORMAT: %q is not one of json, text", v))
		}
	}

	if v := getenv("C4FORGE_SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("C4FORGE_SHUTDOWN_TIMEOUT: %q is not a duration such as 10s", v))
		case d <= 0:
			errs = append(errs, fmt.Errorf("C4FORGE_SHUTDOWN_TIMEOUT: must be positive, got %s", v))
		default:
			cfg.ShutdownTimeout = d
		}
	}

	return cfg, errors.Join(errs...)
}
