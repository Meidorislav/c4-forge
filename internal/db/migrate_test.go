package db

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Meidorislav/c4-forge/internal/db/dbtest"
)

func connectTest(t *testing.T, connURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := Connect(t.Context(), connURL, 10*time.Second, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// latestVersion is the version of the newest embedded migration.
func latestVersion(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	provider, err := newMigrationProvider(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = provider.Close() }()
	sources := provider.ListSources()
	if len(sources) == 0 {
		t.Fatal("no embedded migrations")
	}
	return sources[len(sources)-1].Version
}

func schemaVersion(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	provider, err := newMigrationProvider(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = provider.Close() }()
	v, err := provider.GetDBVersion(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMigrateFreshDatabase(t *testing.T) {
	pool := connectTest(t, dbtest.NewDatabase(t))
	var logs bytes.Buffer
	if err := Migrate(t.Context(), pool, slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	if got, want := schemaVersion(t, pool), latestVersion(t, pool); got != want {
		t.Errorf("schema version = %d, want %d", got, want)
	}
	var trgm bool
	if err := pool.QueryRow(t.Context(),
		"SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')").Scan(&trgm); err != nil {
		t.Fatal(err)
	}
	if !trgm {
		t.Error("pg_trgm extension is not installed")
	}
	if !strings.Contains(logs.String(), "migration applied") {
		t.Errorf("applied migrations were not logged: %s", logs.String())
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	pool := connectTest(t, dbtest.NewDatabase(t))
	log := slog.New(slog.DiscardHandler)
	if err := Migrate(t.Context(), pool, log); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}

	var logs bytes.Buffer
	if err := Migrate(t.Context(), pool, slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if strings.Contains(logs.String(), "migration applied") {
		t.Errorf("second run applied migrations again: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "applied=0") {
		t.Errorf("second run did not report an up-to-date schema: %s", logs.String())
	}
}

func TestMigrateConcurrently(t *testing.T) {
	connURL := dbtest.NewDatabase(t)
	const instances = 4

	// Separate pools stand in for separate application instances.
	pools := make([]*pgxpool.Pool, instances)
	for i := range pools {
		pools[i] = connectTest(t, connURL)
	}

	var wg sync.WaitGroup
	errs := make(chan error, instances)
	for _, pool := range pools {
		wg.Go(func() { errs <- Migrate(t.Context(), pool, slog.New(slog.DiscardHandler)) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Migrate() error = %v", err)
		}
	}
	if got, want := schemaVersion(t, pools[0]), latestVersion(t, pools[0]); got != want {
		t.Errorf("schema version = %d, want %d", got, want)
	}
}

func TestPresentationMigrationPreservesExistingWorkspace(t *testing.T) {
	pool := connectTest(t, dbtest.NewDatabase(t))
	provider, err := newMigrationProvider(pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
	const workspace = "019a2f00-0000-7000-8000-000000000001"
	const element = "019a2f00-0000-7000-8000-000000000002"
	if _, err := pool.Exec(t.Context(), `INSERT INTO workspaces(id,seq) VALUES ($1,7)`, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO elements(workspace_id,id,type,name,dsl_id,attrs) VALUES($1,$2,'softwareSystem','Existing','existing','{"description":"preserve"}')`, workspace, element); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	var seq int64
	var description string
	if err := pool.QueryRow(t.Context(), `SELECT w.seq,e.attrs->>'description' FROM workspaces w JOIN elements e ON e.workspace_id=w.id WHERE w.id=$1`, workspace).Scan(&seq, &description); err != nil {
		t.Fatal(err)
	}
	if seq != 7 || description != "preserve" {
		t.Fatalf("existing data changed: seq=%d description=%q", seq, description)
	}
}
