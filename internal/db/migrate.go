package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Migrations are forward-only (ADR-0003): files have no "Down" sections, and
// every migration must be safe for the data of the previous release. Schema
// changes are SQL files; migrations that need logic are written in Go and
// registered with goose.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies all pending migrations. A PostgreSQL advisory lock ensures
// that only one instance migrates at a time; others wait and then find
// nothing left to do.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	provider, err := newMigrationProvider(pool)
	if err != nil {
		return err
	}
	defer func() { _ = provider.Close() }()

	results, err := provider.Up(ctx)
	for _, r := range results {
		if r.Error != nil {
			continue
		}
		log.Info("migration applied",
			"version", r.Source.Version,
			"file", path.Base(r.Source.Path),
			"duration_ms", float64(r.Duration.Microseconds())/1000,
		)
	}
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	log.Info("database schema is up to date", "version", version, "applied", len(results))
	return nil
}

func newMigrationProvider(pool *pgxpool.Pool) (*goose.Provider, error) {
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	// Instances waiting for another one to finish migrating check the lock
	// every second, for up to five minutes.
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 300))
	if err != nil {
		return nil, fmt.Errorf("create migration lock: %w", err)
	}
	// goose works with database/sql. This adapter borrows connections from
	// the pool; closing it (via Provider.Close) leaves the pool open.
	sqlDB := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, files, goose.WithSessionLocker(locker))
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return provider, nil
}
