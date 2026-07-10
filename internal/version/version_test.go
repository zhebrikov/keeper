package version_test

import (
	"testing"

	"github.com/zhebrikov/gophkeeper/internal/version"
)

func TestVersionDefaults(t *testing.T) {
	if version.Version == "" {
		t.Fatal("expected default version")
	}
	if version.BuildDate == "" {
		t.Fatal("expected default build date")
	}
}
