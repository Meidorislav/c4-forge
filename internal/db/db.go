// Package db manages the PostgreSQL connection pool.
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MinServerVersion is the oldest supported PostgreSQL release, in
// server_version_num form (ADR-0003).
const MinServerVersion = 160000

// Retry timing for Connect.
const (
	firstRetryDelay = 500 * time.Millisecond
	maxRetryDelay   = 5 * time.Second
	attemptTimeout  = 5 * time.Second
)

// Connect opens a connection pool and waits until the server accepts
// connections, retrying transient failures (server still starting, network
// not ready) until timeout elapses. Errors that retrying cannot fix, such as
// wrong credentials or a missing database, fail immediately. It also rejects
// servers older than MinServerVersion.
func Connect(ctx context.Context, url string, timeout time.Duration, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := waitReady(ctx, pool, timeout, log); err != nil {
		pool.Close()
		return nil, err
	}
	if err := checkServerVersion(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func waitReady(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	delay := firstRetryDelay
	for attempt := 1; ; attempt++ {
		err := ping(ctx, pool)
		if err == nil {
			return nil
		}
		if !retryable(err) {
			return fmt.Errorf("connect to database: %w", err)
		}
		log.Warn("database not ready, retrying", "attempt", attempt, "retry_in", delay, "error", err)

		select {
		case <-ctx.Done():
			return fmt.Errorf("connect to database: gave up after %d attempts: %w", attempt, err)
		case <-time.After(delay):
		}
		delay = min(delay*2, maxRetryDelay)
	}
}

func ping(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	return pool.Ping(ctx)
}

// retryable reports whether a failed connection attempt may succeed later.
// The server's own errors are final, except "the database system is starting
// up"; anything else (refused connection, DNS, timeouts) is treated as transient.
func retryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "57P03" // cannot_connect_now
	}
	return true
}

func checkServerVersion(ctx context.Context, pool *pgxpool.Pool) error {
	var num int
	var version string
	err := pool.QueryRow(ctx,
		"SELECT current_setting('server_version_num')::int, current_setting('server_version')",
	).Scan(&num, &version)
	if err != nil {
		return fmt.Errorf("read server version: %w", err)
	}
	return supportedVersion(num, version)
}

func supportedVersion(num int, version string) error {
	if num < MinServerVersion {
		return fmt.Errorf("PostgreSQL %d or newer is required, the server runs %s", MinServerVersion/10000, version)
	}
	return nil
}
