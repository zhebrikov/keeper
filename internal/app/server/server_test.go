package server

import (
	"context"
	"testing"
)

func TestRunMissingConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")

	err := Run(context.Background())
	if err == nil {
		t.Fatal("expected config error")
	}
}
