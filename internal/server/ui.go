package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

const (
	// Vite puts hashed bundles under assets/: a changed file gets a new
	// name, so these can be cached forever.
	assetsDir      = "assets/"
	cacheImmutable = "public, max-age=31536000, immutable"
	// Everything else, index.html in particular, is revalidated so a new
	// release is picked up on the next page load.
	cacheRevalidate = "no-cache"
)

const noUIMessage = "c4-forge: this binary was built without the web UI.\n" +
	"Build it with `make build`, or run the frontend dev server for development.\n"

// uiHandler serves the single-page web UI from files. Existing files are
// served as is; any other path gets index.html so the client-side router can
// handle it, except under assets/, where a missing file is a real 404.
func uiHandler(files fs.FS) http.Handler {
	if files == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(noUIMessage))
		})
	}

	fileServer := http.FileServerFS(files)
	serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(files, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, assetsDir) {
					w.Header().Set("Cache-Control", cacheImmutable)
				} else {
					w.Header().Set("Cache-Control", cacheRevalidate)
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(name, assetsDir) {
				http.NotFound(w, r)
				return
			}
		}
		serveIndex(w, r, files)
	})
	return middleware.Compress(5)(serve)
}

func serveIndex(w http.ResponseWriter, r *http.Request, files fs.FS) {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		http.Error(w, "web UI is missing index.html", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", cacheRevalidate)
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
}
