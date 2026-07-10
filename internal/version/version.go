// Package version holds build-time version information for the client binary.
//
// Version and BuildDate are injected at link time via -ldflags.
package version

// Version is the semantic version of the client, set at build time.
var Version = "dev"

// BuildDate is the UTC timestamp when the client was built, set at build time.
var BuildDate = "unknown"
