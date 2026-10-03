// Package dbtest provides real PostgreSQL databases for integration tests.
//
// A PostgreSQL container is started once per test binary with Testcontainers
// and removed by its reaper when the binary exits. Each call to NewDatabase
// creates a separate database, so tests can run in parallel without sharing
// state.
//
// Without a running Docker daemon the tests are skipped locally. In CI (the CI
// environment variable is set) they fail instead, so integration tests cannot
// silently stop running.
package dbtest

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// DefaultImage is the PostgreSQL image used unless C4FORGE_TEST_POSTGRES_IMAGE
// overrides it (CI uses that to test every supported major version).
const DefaultImage = "postgres:18"

var (
	startOnce sync.Once
	adminURL  string
	startErr  error
)

// NewDatabase returns a connection string for a new, empty database that is
// dropped when the test ends.
func NewDatabase(t *testing.T) string {
	t.Helper()
	server := ServerURL(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	name := "test_" + strings.ToLower(rand.Text())
	if err := adminExec(ctx, server, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("dbtest: create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := adminExec(ctx, server, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("dbtest: drop database %s: %v", name, err)
		}
	})
	return WithDatabase(t, server, name)
}

// ServerURL returns a connection string for the shared test server's
// maintenance database, starting the server on first use.
func ServerURL(t *testing.T) string {
	t.Helper()
	if os.Getenv("CI") == "" {
		testcontainers.SkipIfProviderIsNotHealthy(t)
	}
	startOnce.Do(func() { adminURL, startErr = start() })
	if startErr != nil {
		t.Fatalf("dbtest: start PostgreSQL: %v", startErr)
	}
	return adminURL
}

// WithDatabase returns connURL pointing at database name instead.
func WithDatabase(t *testing.T, connURL, name string) string {
	t.Helper()
	u, err := url.Parse(connURL)
	if err != nil {
		t.Fatalf("dbtest: parse connection string: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func start() (string, error) {
	image := os.Getenv("C4FORGE_TEST_POSTGRES_IMAGE")
	if image == "" {
		image = DefaultImage
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ctr, err := postgres.Run(ctx, image,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("c4forge"),
		postgres.WithPassword("c4forge"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", fmt.Errorf("run %s: %w", image, err)
	}
	return ctr.ConnectionString(ctx, "sslmode=disable")
}

func adminExec(ctx context.Context, connURL, sql string) error {
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	_, err = conn.Exec(ctx, sql)
	return err
}
