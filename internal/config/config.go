// Package config provides application configuration loading from environment variables.
//
// Server configuration ([LoadServer]) requires:
//   - DATABASE_URL — PostgreSQL connection string
//   - JWT_SECRET — HMAC secret for signing access tokens
//
// Optional server variables: GRPC_ADDR, HTTP_ADDR, ACCESS_TOKEN_TTL, REFRESH_TOKEN_TTL,
// TLS_CERT, TLS_KEY (both required to enable TLS on the gRPC server).
//
// Client configuration ([LoadClient]) reads:
//   - GOPHKEEPER_SERVER — gRPC server address (default localhost:8080)
//   - GOPHKEEPER_CONFIG — local config directory (default ~/.gophkeeper)
//   - GOPHKEEPER_TLS — enable TLS (true/1)
//   - GOPHKEEPER_TLS_CA — path to CA certificate for server verification
//   - GOPHKEEPER_INSECURE — disable TLS (development only)
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultGRPCAddr        = ":8080"
	defaultHTTPAddr        = ":8090"
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 7 * 24 * time.Hour
)

// ServerConfig holds server-side configuration values.
type ServerConfig struct {
	// DatabaseURL is the PostgreSQL connection string (required).
	DatabaseURL string
	// GRPCAddr is the listen address for the gRPC server.
	GRPCAddr string
	// HTTPAddr is the listen address for REST API and Swagger UI.
	HTTPAddr string
	// JWTSecret is the HMAC secret used to sign access tokens (required).
	JWTSecret string
	// AccessTokenTTL is the lifetime of issued access tokens.
	AccessTokenTTL time.Duration
	// RefreshTokenTTL is the lifetime of issued refresh tokens.
	RefreshTokenTTL time.Duration
	// TLSCertFile is the path to the TLS certificate file.
	TLSCertFile string
	// TLSKeyFile is the path to the TLS private key file.
	TLSKeyFile string
}

// ClientConfig holds client-side configuration values.
type ClientConfig struct {
	// ServerAddr is the gRPC address of the GophKeeper server.
	ServerAddr string
	// ConfigPath is the directory for local client configuration and session data.
	ConfigPath string
	// TLSEnabled reports whether the client should use TLS.
	TLSEnabled bool
	// TLSCAFile is the path to the CA certificate used to verify the server.
	TLSCAFile string
	// Insecure disables TLS and should only be used in local development.
	Insecure bool
}

// LoadServer reads server configuration from environment variables.
func LoadServer() (ServerConfig, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return ServerConfig{}, fmt.Errorf("DATABASE_URL is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return ServerConfig{}, fmt.Errorf("JWT_SECRET is required")
	}

	tlsCert := os.Getenv("TLS_CERT")
	tlsKey := os.Getenv("TLS_KEY")
	if (tlsCert == "") != (tlsKey == "") {
		return ServerConfig{}, fmt.Errorf("TLS_CERT and TLS_KEY must both be set to enable TLS")
	}

	cfg := ServerConfig{
		DatabaseURL:     dbURL,
		GRPCAddr:        envOrDefault("GRPC_ADDR", defaultGRPCAddr),
		HTTPAddr:        envOrDefault("HTTP_ADDR", defaultHTTPAddr),
		JWTSecret:       jwtSecret,
		AccessTokenTTL:  parseDuration("ACCESS_TOKEN_TTL", defaultAccessTokenTTL),
		RefreshTokenTTL: parseDuration("REFRESH_TOKEN_TTL", defaultRefreshTokenTTL),
		TLSCertFile:     tlsCert,
		TLSKeyFile:      tlsKey,
	}

	return cfg, nil
}

// LoadClient reads client configuration from environment variables.
func LoadClient() ClientConfig {
	return ClientConfig{
		ServerAddr: envOrDefault("GOPHKEEPER_SERVER", "localhost:8080"),
		ConfigPath: envOrDefault("GOPHKEEPER_CONFIG", defaultConfigPath()),
		TLSEnabled: envBool("GOPHKEEPER_TLS"),
		TLSCAFile:  os.Getenv("GOPHKEEPER_TLS_CA"),
		Insecure:   envBool("GOPHKEEPER_INSECURE"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}

	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}

	return fallback
}
