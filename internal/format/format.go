// Package format provides shared formatting helpers for CLI and TUI.
package format

import (
	"time"
)

// UnixTimestamp formats a Unix timestamp for display.
// Zero timestamps are shown as "-".
func UnixTimestamp(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02 15:04")
}

// Truncate shortens s to max runes, appending "..." when truncated.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
