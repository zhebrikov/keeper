package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/zhebrikov/gophkeeper/internal/config"
)

func TestLoadServerMissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")

	_, err := config.LoadServer()
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadServerSuccess(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("GRPC_ADDR", ":9090")
	t.Setenv("ACCESS_TOKEN_TTL", "30m")

	cfg, err := config.LoadServer()
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Fatalf("unexpected database url: %s", cfg.DatabaseURL)
	}
	if cfg.GRPCAddr != ":9090" {
		t.Fatalf("unexpected grpc addr: %s", cfg.GRPCAddr)
	}
	if cfg.AccessTokenTTL != 30*time.Minute {
		t.Fatalf("unexpected access ttl: %v", cfg.AccessTokenTTL)
	}
}

func TestLoadClientDefaults(t *testing.T) {
	os.Unsetenv("GOPHKEEPER_SERVER")
	os.Unsetenv("GOPHKEEPER_CONFIG")

	cfg := config.LoadClient()
	if cfg.ServerAddr != "localhost:8080" {
		t.Fatalf("unexpected server addr: %s", cfg.ServerAddr)
	}
	if cfg.ConfigPath == "" {
		t.Fatal("expected non-empty config path")
	}
}

func TestLoadServerTLSRequiresBothFiles(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("TLS_CERT", "server.crt")
	t.Setenv("TLS_KEY", "")

	_, err := config.LoadServer()
	if err == nil {
		t.Fatal("expected error when only TLS_CERT is set")
	}
}

func TestLoadServerTLSConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("TLS_CERT", "server.crt")
	t.Setenv("TLS_KEY", "server.key")

	cfg, err := config.LoadServer()
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.TLSCertFile != "server.crt" || cfg.TLSKeyFile != "server.key" {
		t.Fatalf("unexpected TLS config: %+v", cfg)
	}
}

func TestLoadClientTLS(t *testing.T) {
	t.Setenv("GOPHKEEPER_TLS", "true")
	t.Setenv("GOPHKEEPER_TLS_CA", "ca.crt")
	t.Setenv("GOPHKEEPER_INSECURE", "false")

	cfg := config.LoadClient()
	if !cfg.TLSEnabled {
		t.Fatal("expected TLS to be enabled")
	}
	if cfg.TLSCAFile != "ca.crt" {
		t.Fatalf("unexpected CA file: %s", cfg.TLSCAFile)
	}
}

func TestLoadClientInsecure(t *testing.T) {
	t.Setenv("GOPHKEEPER_INSECURE", "1")

	cfg := config.LoadClient()
	if !cfg.Insecure {
		t.Fatal("expected insecure mode")
	}
}

func TestLoadServerRefreshTokenTTLSeconds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("REFRESH_TOKEN_TTL", "3600")

	cfg, err := config.LoadServer()
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if cfg.RefreshTokenTTL != time.Hour {
		t.Fatalf("unexpected refresh ttl: %v", cfg.RefreshTokenTTL)
	}
}
