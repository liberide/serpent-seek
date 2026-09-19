// Package version holds build metadata injected at link time.
package version

// Version is the semantic version of the binary. It can be overridden with
// -ldflags "-X github.com/liberide/serpent-seek/internal/version.Version=x.y.z".
var (
	// Version is the application version string (semver).
	Version = "1.0.2"
	// Commit is the VCS revision the binary was built from.
	Commit = "dev"
	// BuildDate is the RFC3339 timestamp of the build.
	BuildDate = "unknown"
)

// String returns a compact human readable version line.
func String() string {
	return Version + " (" + Commit + ", " + BuildDate + ")"
}
