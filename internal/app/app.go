// Package app assembles the c4-forge service from its parts and runs it.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/Meidorislav/c4-forge/internal/buildinfo"
	"github.com/Meidorislav/c4-forge/internal/config"
	"github.com/Meidorislav/c4-forge/internal/db"
	"github.com/Meidorislav/c4-forge/internal/server"
)

// Exit codes returned by Run.
const (
	ExitOK     = 0
	ExitError  = 1
	ExitConfig = 2
)

// dbConnectTimeout bounds how long startup waits for PostgreSQL, e.g. while
// both containers are still starting.
const dbConnectTimeout = 30 * time.Second

// Run parses args, loads the configuration via getenv and serves until ctx is
// cancelled. It returns a process exit code. Run touches no process-global
// state, so it can be tested.
func Run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("c4forge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitConfig
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildinfo.String())
		return ExitOK
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "c4forge: invalid configuration:\n%v\n", err)
		return ExitConfig
	}
	log := newLogger(stdout, cfg)

	pool, err := db.Connect(ctx, cfg.DatabaseURL, dbConnectTimeout, log)
	if err != nil {
		log.Error("cannot connect to the database", "error", err)
		return ExitError
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, log); err != nil {
		log.Error("cannot migrate the database", "error", err)
		return ExitError
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		log.Error("cannot listen", "addr", cfg.HTTPAddr, "error", err)
		return ExitError
	}
	log.Info("c4forge started",
		"addr", ln.Addr().String(),
		"version", buildinfo.Version,
		"revision", buildinfo.Revision(),
	)

	srv := server.New(log, cfg.ShutdownTimeout,
		server.ReadinessCheck{Name: "database", Check: pool.Ping},
	)
	if err := srv.Serve(ctx, ln); err != nil {
		log.Error("server stopped with an error", "error", err)
		return ExitError
	}
	log.Info("c4forge stopped")
	return ExitOK
}

func newLogger(w io.Writer, cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == config.LogFormatText {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
