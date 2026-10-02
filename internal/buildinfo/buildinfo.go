// Package buildinfo reports the version and commit the binary was built from.
package buildinfo

import "runtime/debug"

// Version and Commit are set at build time via -ldflags "-X ...".
var (
	Version = "dev"
	Commit  = ""
)

// Revision returns the commit the binary was built from. It prefers the value
// injected via -ldflags and falls back to the VCS information that the Go
// toolchain embeds when building inside a git checkout.
func Revision() string {
	if Commit != "" {
		return Commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

// String returns a one-line description such as "c4forge dev (a1b2c3d4e5f6)".
func String() string {
	return "c4forge " + Version + " (" + Revision() + ")"
}
