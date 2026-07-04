// Package build holds version information injected at build time via ldflags.
package build

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
	BuiltBy = "unknown"
)
