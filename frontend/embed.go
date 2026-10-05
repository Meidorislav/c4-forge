// Package frontend exposes the built web UI to the Go server.
//
// The UI is embedded only when building with the "ui" tag (make build does
// this after building the frontend). Without the tag, Files returns nil and
// the server runs API-only, so backend development and tests need no Node.js.
package frontend
