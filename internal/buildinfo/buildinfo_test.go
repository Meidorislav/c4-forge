package buildinfo

import (
	"strings"
	"testing"
)

func TestRevisionPrefersInjectedCommit(t *testing.T) {
	old := Commit
	t.Cleanup(func() { Commit = old })

	Commit = "abc123"
	if got := Revision(); got != "abc123" {
		t.Errorf("Revision() = %q, want injected commit", got)
	}
}

func TestString(t *testing.T) {
	oldVersion, oldCommit := Version, Commit
	t.Cleanup(func() { Version, Commit = oldVersion, oldCommit })

	Version, Commit = "v1.2.3", "abc123"
	if got, want := String(), "c4forge v1.2.3 (abc123)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestRevisionFallbackIsNotEmpty(t *testing.T) {
	old := Commit
	t.Cleanup(func() { Commit = old })

	Commit = ""
	if got := Revision(); got == "" || strings.ContainsAny(got, " \n") {
		t.Errorf("Revision() = %q, want a non-empty single token", got)
	}
}
