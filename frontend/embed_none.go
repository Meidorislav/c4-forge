//go:build !ui

package frontend

import "io/fs"

// Files returns nil: this binary was built without the UI.
func Files() fs.FS { return nil }
