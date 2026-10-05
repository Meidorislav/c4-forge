//go:build ui

package frontend

import (
	"io/fs"
	"testing"
)

func TestFilesContainIndex(t *testing.T) {
	if _, err := fs.Stat(Files(), "index.html"); err != nil {
		t.Fatalf("embedded UI has no index.html: %v", err)
	}
}
