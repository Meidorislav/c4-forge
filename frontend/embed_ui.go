//go:build ui

package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Files returns the built UI (the contents of frontend/dist).
func Files() fs.FS {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		// The embed directive guarantees dist exists.
		panic(err)
	}
	return files
}
