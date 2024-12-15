package secrets

import (
	"context"
)

// Source represents different sources of secrets.
type Source uint8

const (
	// EnvFile indicates secrets should be loaded from .env file.
	EnvFile Source = iota
	// EnvVars indicates secrets should be loaded from environment variables.
	EnvVars
	// Both indicates secrets should be loaded from both .env file and environment variables.
	Both
)

// Provider defines the interface for secret management implementations.
type Provider interface {
	// GetSecret retrieves a secret by its key
	GetSecret(ctx context.Context, key string) (string, error)
	// SetSecret stores a secret with the given key and value
	SetSecret(ctx context.Context, key, value string) error
}

// Config holds configuration options for secret providers.
type Config struct {
	// Source determines where to load secrets from
	Source Source
	// Prefix is used to namespace environment variables
	Prefix string
	// EnvPath is the path to the .env file
	EnvPath string
	// AllowMissing determines if missing secrets should cause an error
	AllowMissing bool
}

// Store represents a collection of secrets with their metadata.
type Store struct {
	// DBCredentials holds database access information
	DBCredentials struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	} `mapstructure:"db_credentials"`
	// APIKeys holds various API authentication keys
	APIKeys struct {
		// Add API keys here
	} `mapstructure:"api_keys"`
}
