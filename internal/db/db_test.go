package db

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Meidorislav/c4-forge/internal/db/dbtest"
)

func TestSupportedVersion(t *testing.T) {
	if err := supportedVersion(160000, "16.0"); err != nil {
		t.Errorf("16.0 rejected: %v", err)
	}
	if err := supportedVersion(180006, "18.6"); err != nil {
		t.Errorf("18.6 rejected: %v", err)
	}
	err := supportedVersion(150019, "15.19")
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL 16 or newer") || !strings.Contains(err.Error(), "15.19") {
		t.Errorf("15.19: error = %v, want a clear version error", err)
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"server starting up", &pgconn.PgError{Code: "57P03"}, true},
		{"wrong password", &pgconn.PgError{Code: "28P01"}, false},
		{"missing database", &pgconn.PgError{Code: "3D000"}, false},
		{"network error", errors.New("dial tcp: connection refused"), true},
	}
	for _, tt := range tests {
		if got := retryable(tt.err); got != tt.want {
			t.Errorf("%s: retryable() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestConnectGivesUpAfterTimeout(t *testing.T) {
	// A port that was just free: connections are refused, which is retryable.
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	start := time.Now()
	_, err = Connect(t.Context(), "postgres://c4forge:c4forge@"+addr+"/c4forge?sslmode=disable", 1200*time.Millisecond, log)
	if err == nil {
		t.Fatal("Connect() succeeded against a closed port")
	}
	if !strings.Contains(err.Error(), "gave up after") {
		t.Errorf("error = %v, want it to report giving up", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Connect() took %s, want about the 1.2s timeout", elapsed)
	}
	if !strings.Contains(logs.String(), "database not ready, retrying") {
		t.Errorf("no retry was logged: %s", logs.String())
	}
}

func TestConnect(t *testing.T) {
	connURL := dbtest.NewDatabase(t)
	pool, err := Connect(t.Context(), connURL, 10*time.Second, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer pool.Close()

	var one int
	if err := pool.QueryRow(t.Context(), "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("query on the pool: %v", err)
	}
}

func TestConnectFailsFastOnPermanentErrors(t *testing.T) {
	server := dbtest.ServerURL(t)

	wrongPassword, err := url.Parse(server)
	if err != nil {
		t.Fatal(err)
	}
	wrongPassword.User = url.UserPassword("c4forge", "leak-probe-x9")

	tests := map[string]string{
		"wrong password":   wrongPassword.String(),
		"missing database": dbtest.WithDatabase(t, server, "does_not_exist"),
	}
	for name, connURL := range tests {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			_, err := Connect(t.Context(), connURL, 30*time.Second, slog.New(slog.DiscardHandler))
			if err == nil {
				t.Fatal("Connect() succeeded")
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Errorf("Connect() took %s; permanent errors must not be retried", elapsed)
			}
			if strings.Contains(err.Error(), "leak-probe-x9") {
				t.Errorf("error leaks the password: %v", err)
			}
		})
	}
}
