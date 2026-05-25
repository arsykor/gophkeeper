// Package config loads server configuration from environment variables and flags.
package config

import (
	"flag"

	"github.com/caarlos0/env/v6"
)

// Config holds all server settings.
type Config struct {
	// GRPCAddress is the address the gRPC server listens on.
	GRPCAddress string `env:"GRPC_ADDRESS"`

	// DatabaseDSN is the Postgres connection string.
	DatabaseDSN string `env:"DATABASE_DSN"`

	// MinioEndpoint is the MinIO server host:port.
	MinioEndpoint string `env:"MINIO_ENDPOINT"`
	// MinioAccessKey is the MinIO access key ID.
	MinioAccessKey string `env:"MINIO_ACCESS_KEY"`
	// MinioSecretKey is the MinIO secret access key.
	MinioSecretKey string `env:"MINIO_SECRET_KEY"`
	// MinioBucket is the bucket used for binary secrets.
	MinioBucket string `env:"MINIO_BUCKET"`
	// MinioUseSSL enables TLS for MinIO connections.
	MinioUseSSL bool `env:"MINIO_USE_SSL"`

	// JWTSecret is the HMAC secret used to sign JWT tokens.
	JWTSecret string `env:"JWT_SECRET"`

	// EncryptionKey is a 32-byte hex string used for AES-256-GCM encryption.
	EncryptionKey string `env:"ENCRYPTION_KEY"`
}

// Load parses environment variables and then applies command-line flags.
func Load() (*Config, error) {
	cfg := &Config{
		GRPCAddress:    ":50051",
		MinioBucket:    "gophkeeper",
		MinioEndpoint:  "localhost:9000",
		MinioAccessKey: "minioadmin",
		MinioSecretKey: "minioadmin",
	}

	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	flag.StringVar(&cfg.GRPCAddress, "a", cfg.GRPCAddress, "gRPC listen address")
	flag.StringVar(&cfg.DatabaseDSN, "d", cfg.DatabaseDSN, "Postgres DSN")
	flag.StringVar(&cfg.JWTSecret, "jwt", cfg.JWTSecret, "JWT HMAC secret")
	flag.StringVar(&cfg.EncryptionKey, "enc", cfg.EncryptionKey, "AES-256 encryption key (32 hex bytes)")
	flag.Parse()

	return cfg, nil
}
