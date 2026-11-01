// Package config loads the service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LogFormat selects how log records are encoded.
type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// Config is the complete service configuration.
type Config struct {
	PublicOrigin        string
	BootstrapSecret     string
	InsecureDevelopment bool
	SessionIdle         time.Duration
	SessionAbsolute     time.Duration
	// HTTPAddr is the address the HTTP server listens on (C4FORGE_HTTP_ADDR).
	HTTPAddr string
	// LogLevel is the minimum level of emitted log records (C4FORGE_LOG_LEVEL).
	LogLevel slog.Level
	// LogFormat is the log encoding (C4FORGE_LOG_FORMAT).
	LogFormat LogFormat
	// ShutdownTimeout bounds how long in-flight requests may take to finish
	// after a shutdown signal (C4FORGE_SHUTDOWN_TIMEOUT).
	ShutdownTimeout time.Duration
	// DatabaseURL is the PostgreSQL connection string, as a URL or in
	// keyword/value form (C4FORGE_DATABASE_URL). Required. Pool settings such
	// as pool_max_conns can be passed as parameters. It contains credentials
	// and must never be logged.
	DatabaseURL string
}

// Default returns the defaults for optional settings. Required settings, such
// as DatabaseURL, are left empty.
func Default() Config {
	return Config{
		HTTPAddr:        ":8080",
		PublicOrigin:    "https://localhost:8080",
		SessionIdle:     30 * time.Minute,
		SessionAbsolute: 12 * time.Hour,
		LogLevel:        slog.LevelInfo,
		LogFormat:       LogFormatJSON,
		ShutdownTimeout: 10 * time.Second,
	}
}

// Load reads the configuration using getenv (usually os.Getenv). Unset or
// empty optional variables keep their defaults; missing required ones are
// errors. All problems are reported together.
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

	if v := getenv("C4FORGE_DATABASE_URL"); v == "" {
		errs = append(errs, errors.New("C4FORGE_DATABASE_URL: required, e.g. postgres://user:password@localhost:5432/c4forge"))
	} else if _, err := pgxpool.ParseConfig(v); err != nil {
		// pgx masks the password in its parse errors.
		errs = append(errs, fmt.Errorf("C4FORGE_DATABASE_URL: %w", err))
	} else {
		cfg.DatabaseURL = v
	}

	if v := getenv("C4FORGE_PUBLIC_ORIGIN"); v != "" {
		cfg.PublicOrigin = v
	}
	cfg.BootstrapSecret = getenv("C4FORGE_BOOTSTRAP_SECRET")
	if cfg.BootstrapSecret != "" && len(cfg.BootstrapSecret) < 32 {
		errs = append(errs, errors.New("C4FORGE_BOOTSTRAP_SECRET must contain at least 32 bytes"))
	}
	if v := getenv("C4FORGE_INSECURE_DEV_MODE"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, errors.New("C4FORGE_INSECURE_DEV_MODE must be true or false"))
		} else {
			cfg.InsecureDevelopment = parsed
		}
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{{"C4FORGE_SESSION_IDLE", &cfg.SessionIdle}, {"C4FORGE_SESSION_ABSOLUTE", &cfg.SessionAbsolute}} {
		if v := getenv(setting.name); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d < time.Second || d > 365*24*time.Hour {
				errs = append(errs, fmt.Errorf("%s must be between 1s and 8760h", setting.name))
			} else {
				*setting.target = d
			}
		}
	}
	if cfg.SessionIdle > cfg.SessionAbsolute {
		errs = append(errs, errors.New("C4FORGE_SESSION_IDLE must not exceed C4FORGE_SESSION_ABSOLUTE"))
	}
	origin, err := url.Parse(cfg.PublicOrigin)
	if err != nil || origin == nil || origin.User != nil || origin.Hostname() == "" || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		errs = append(errs, errors.New("C4FORGE_PUBLIC_ORIGIN must be an origin with scheme, host and optional port"))
	} else {
		hostname := strings.ToLower(origin.Hostname())
		port := origin.Port()
		if port != "" {
			portNumber, portErr := strconv.Atoi(port)
			if portErr != nil || portNumber < 1 || portNumber > 65535 {
				errs = append(errs, errors.New("C4FORGE_PUBLIC_ORIGIN has an invalid port"))
			}
		}
		if (origin.Scheme == "https" && port == "443") || (origin.Scheme == "http" && port == "80") {
			port = ""
		}
		host := hostname
		if port != "" {
			host = net.JoinHostPort(hostname, port)
		} else if strings.Contains(hostname, ":") {
			host = "[" + hostname + "]"
		}
		cfg.PublicOrigin = origin.Scheme + "://" + host
		if cfg.InsecureDevelopment {
			host, _, _ := net.SplitHostPort(cfg.HTTPAddr)
			if origin.Scheme != "http" || !loopbackHost(origin.Hostname()) || !loopbackHost(host) {
				errs = append(errs, errors.New("insecure development mode requires an HTTP loopback origin and loopback listener"))
			}
		} else if origin.Scheme != "https" {
			errs = append(errs, errors.New("C4FORGE_PUBLIC_ORIGIN must use HTTPS outside insecure development mode"))
		}
	}
	return cfg, errors.Join(errs...)
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
