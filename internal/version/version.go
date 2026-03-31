package version

import "fmt"

// These are set at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a formatted version string.
func String() string {
	return fmt.Sprintf("pqpm %s (commit: %s, built: %s)", Version, Commit, Date)
}
