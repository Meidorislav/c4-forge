package server

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	testIndex  = "<!doctype html><title>c4-forge</title><div id=root></div>"
	testScript = "console.log('app');"
)

var testUI = fstest.MapFS{
	"index.html":           {Data: []byte(testIndex)},
	"favicon.svg":          {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")},
	"assets/index-abc.js":  {Data: []byte(testScript + strings.Repeat(" ", 2048))},
	"assets/index-abc.css": {Data: []byte("body{margin:0}")},
}

func serveUI(t *testing.T, ui Options, method, target string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(slog.New(slog.DiscardHandler), ui)
	req := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUIServesIndexForAppRoutes(t *testing.T) {
	for _, target := range []string{"/", "/index.html", "/workspaces/123", "/w/a.b/view"} {
		t.Run(target, func(t *testing.T) {
			rec := serveUI(t, Options{UI: testUI}, http.MethodGet, target, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "<div id=root>") {
				t.Errorf("body = %q, want index.html", rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type = %q, want text/html", ct)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != cacheRevalidate {
				t.Errorf("Cache-Control = %q, want %q", cc, cacheRevalidate)
			}
		})
	}
}

func TestUIServesHashedAssetsImmutable(t *testing.T) {
	rec := serveUI(t, Options{UI: testUI}, http.MethodGet, "/assets/index-abc.js", nil)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), testScript) {
		t.Fatalf("got %d %q, want the script", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want JavaScript", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != cacheImmutable {
		t.Errorf("Cache-Control = %q, want %q", cc, cacheImmutable)
	}
}

func TestUIServesOtherFilesWithRevalidation(t *testing.T) {
	rec := serveUI(t, Options{UI: testUI}, http.MethodGet, "/favicon.svg", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != cacheRevalidate {
		t.Errorf("Cache-Control = %q, want %q", cc, cacheRevalidate)
	}
}

func TestUIMissingAssetIs404(t *testing.T) {
	// A stale page asking for a bundle from an old release must get a 404,
	// not index.html served as JavaScript.
	rec := serveUI(t, Options{UI: testUI}, http.MethodGet, "/assets/index-old.js", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUICompressesResponses(t *testing.T) {
	rec := serveUI(t, Options{UI: testUI}, http.MethodGet, "/assets/index-abc.js",
		map[string]string{"Accept-Encoding": "gzip"})
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), testScript) {
		t.Errorf("decompressed body = %q", body)
	}
}

func TestUIRejectsWrites(t *testing.T) {
	rec := serveUI(t, Options{UI: testUI}, http.MethodPost, "/workspaces", nil)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("got %d Allow=%q, want 405 with Allow: GET, HEAD", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	for _, target := range []string{"/api", "/api/", "/api/nope"} {
		rec := serveUI(t, Options{UI: testUI}, http.MethodGet, target, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type = %q, want application/json", target, ct)
		}
	}
}

func TestWithoutUI(t *testing.T) {
	rec := serveUI(t, Options{}, http.MethodGet, "/", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "built without the web UI") {
		t.Errorf("got %d %q, want a 404 explaining the UI is not included", rec.Code, rec.Body.String())
	}
	// The API keeps working.
	if rec := serveUI(t, Options{}, http.MethodGet, "/healthz", nil); rec.Code != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	for _, target := range []string{"/", "/assets/index-abc.js", "/healthz", "/api/nope"} {
		rec := serveUI(t, Options{UI: testUI}, http.MethodGet, target, nil)
		h := rec.Header()
		csp := h.Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "connect-src 'self'") {
			t.Errorf("%s: Content-Security-Policy = %q", target, csp)
		}
		for name, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		} {
			if got := h.Get(name); got != want {
				t.Errorf("%s: %s = %q, want %q", target, name, got, want)
			}
		}
	}
}
