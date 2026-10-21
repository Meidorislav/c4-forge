package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Meidorislav/c4-forge/internal/config"
)

// healthcheckTimeout bounds the whole probe. A variable so tests can shorten it.
var healthcheckTimeout = 3 * time.Second

// healthcheck asks the running server on this host whether it is ready, for
// container health checks (the runtime image has no curl). It exits 0 when
// /readyz answers 200 and 1 otherwise, printing the reason.
func healthcheck(ctx context.Context, cfg config.Config, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	url := "http://" + probeAddr(cfg.HTTPAddr) + "/readyz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(stderr, "c4forge healthcheck: %v\n", err)
		return ExitError
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(stderr, "c4forge healthcheck: %v\n", err)
		return ExitError
	}
	defer func() { _ = res.Body.Close() }()
	// /readyz names the failing checks; show them in `docker inspect`.
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	if res.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "c4forge healthcheck: %s: %s %s\n", url, res.Status, strings.TrimSpace(string(body)))
		return ExitError
	}
	fmt.Fprintln(stdout, "ok")
	return ExitOK
}

// probeAddr turns the server's listen address into one to connect to: a
// wildcard or empty host means "all interfaces", which is reachable on the
// IPv4 loopback.
func probeAddr(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return listenAddr
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
