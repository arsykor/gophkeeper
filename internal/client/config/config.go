// Package config loads client configuration from environment variables and flags.
package config

import (
	"flag"

	"github.com/caarlos0/env/v6"
)

// Config holds client settings.
type Config struct {
	// ServerAddress is the host:port of the gRPC server.
	ServerAddress string `env:"SERVER_ADDRESS"`
	// Insecure disables TLS (for local development).
	Insecure bool `env:"INSECURE"`
}

// Load parses environment variables then applies CLI flags.
func Load() (*Config, error) {
	cfg := &Config{
		ServerAddress: "localhost:50051",
		Insecure:      true,
	}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	flag.StringVar(&cfg.ServerAddress, "s", cfg.ServerAddress, "gRPC server address")
	flag.BoolVar(&cfg.Insecure, "insecure", cfg.Insecure, "disable TLS")
	flag.Parse()
	return cfg, nil
}
