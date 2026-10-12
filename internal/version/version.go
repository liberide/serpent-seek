// Package version holds build metadata injected at link time.
package version

import (
	"runtime/debug"
	"sync"
)

// Build metadata. Version is the application semver; Commit and BuildDate are
// injected by the Makefile/Docker build via
// -ldflags "-X github.com/liberide/serpent-seek/internal/version.Version=x.y.z".
var (
	// Version is the application version string (semver).
	Version = "1.1.1"
	// Commit is the VCS revision the binary was built from.
	Commit = "dev"
	// BuildDate is the RFC3339 timestamp of the build.
	BuildDate = "unknown"
)

var (
	metaOnce  sync.Once
	commit    string
	buildDate string
)

// metadata resolves the injected values, falling back to the VCS metadata the
// Go toolchain embeds for a plain `go build`/`go run` in a git checkout, so a
// local build shows a real commit instead of the "dev" placeholder.
func metadata() (string, string) {
	metaOnce.Do(func() {
		commit, buildDate = Commit, BuildDate
		if commit != "" && commit != "dev" && buildDate != "" && buildDate != "unknown" {
			return
		}
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if commit == "" || commit == "dev" {
					if len(s.Value) > 7 {
						commit = s.Value[:7]
					} else {
						commit = s.Value
					}
				}
			case "vcs.time":
				if buildDate == "" || buildDate == "unknown" {
					buildDate = s.Value
				}
			}
		}
	})
	return commit, buildDate
}

// String returns a compact human readable version line, e.g.
// "1.1.0 (d412056, 2026-10-09T19:53:00Z)".
func String() string {
	c, d := metadata()
	return Version + " (" + c + ", " + d + ")"
}

// Revision returns the effective commit hash (injected via ldflags or taken
// from the VCS metadata embedded by the Go toolchain).
func Revision() string {
	c, _ := metadata()
	return c
}

// BuiltAt returns the effective build timestamp (injected or VCS-derived).
func BuiltAt() string {
	_, d := metadata()
	return d
}
