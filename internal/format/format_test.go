package format

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUnixTimestamp(t *testing.T) {
	require.Equal(t, "-", UnixTimestamp(0))
	require.Equal(t,
		time.Unix(12345, 0).UTC().Format("2006-01-02 15:04"),
		UnixTimestamp(12345),
	)
}

func TestTruncate(t *testing.T) {
	require.Equal(t, "hello", Truncate("hello", 10))
	require.Equal(t, "hel...", Truncate("hello world", 6))
}
