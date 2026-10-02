// Command c4forge runs the c4-forge server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/Meidorislav/c4-forge/internal/buildinfo"
	"github.com/Meidorislav/c4-forge/internal/config"
	"github.com/Meidorislav/c4-forge/internal/server"
)

// Exit codes.
const (
	exitOK     = 0
	exitError  = 1
	exitConfig = 2
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run is main without process-global state, so it can be tested. It serves
// until ctx is cancelled.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("c4forge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitConfig
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildinfo.String())
		return exitOK
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "c4forge: invalid configuration:\n%v\n", err)
		return exitConfig
	}
	log := newLogger(stdout, cfg)

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		log.Error("cannot listen", "addr", cfg.HTTPAddr, "error", err)
		return exitError
	}
	log.Info("c4forge started",
		"addr", ln.Addr().String(),
		"version", buildinfo.Version,
		"revision", buildinfo.Revision(),
	)

	if err := server.New(log, cfg.ShutdownTimeout).Serve(ctx, ln); err != nil {
		log.Error("server stopped with an error", "error", err)
		return exitError
	}
	log.Info("c4forge stopped")
	return exitOK
}

func newLogger(w io.Writer, cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == config.LogFormatText {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
