package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zhebrikov/gophkeeper/internal/client"
	"github.com/zhebrikov/gophkeeper/internal/config"
)

func TestNewRequiresTLSCA(t *testing.T) {
	_, err := client.New(config.ClientConfig{
		ServerAddr: "localhost:8080",
		ConfigPath: t.TempDir(),
		TLSEnabled: true,
	})
	require.Error(t, err)
}

func TestNewInsecureDoesNotRequireCA(t *testing.T) {
	_, err := client.New(config.ClientConfig{
		ServerAddr: "localhost:1",
		ConfigPath: t.TempDir(),
		Insecure:   true,
	})
	require.NoError(t, err)
}
