package dotenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hyp3rd/base/internal/secrets"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
	"github.com/joho/godotenv"
)

// Provider is a struct that represents a DotEnv secret provider. It holds the configuration
// for the provider and manages the loading and access to secrets from a .env file.
type Provider struct {
	config secrets.Config
	mu     sync.RWMutex
	loaded bool
}

// New creates a new DotEnv secret provider with the given configuration.
// If the configuration specifies the EnvVars source, an error is returned
// as the DotEnv provider is not applicable for that source. If the EnvPath
// is not set, it defaults to ".env". The provider's environment file path
// is validated and an error is returned if the file does not exist.
func New(config secrets.Config) (*Provider, error) {
	if config.Source == secrets.EnvVars {
		return nil, ewrap.New("invalid configuration: EnvVars source doesn't require DotEnv provider")
	}

	if config.EnvPath == "" {
		config.EnvPath = ".env"
	}

	provider := &Provider{
		config: config,
	}

	if err := provider.validateEnvPath(); err != nil {
		return nil, err
	}

	return provider, nil
}

func (p *Provider) validateEnvPath() error {
	absPath, err := filepath.Abs(p.config.EnvPath)
	if err != nil {
		return ewrap.Wrapf(err, "resolving env file path").
			WithMetadata("path", p.config.EnvPath)
	}

	p.config.EnvPath = absPath

	if p.config.Source == secrets.EnvFile {
		if _, err := os.Stat(absPath); err != nil {
			if os.IsNotExist(err) {
				return ewrap.New("env file not found").
					WithMetadata("path", absPath)
			}

			return ewrap.Wrapf(err, "checking env file").
				WithMetadata("path", absPath)
		}
	}

	return nil
}

// GetSecret retrieves the value of the secret with the given key from the
// DotEnv provider. If the secret is not found and the provider's configuration
// does not allow missing secrets, an error is returned.
func (p *Provider) GetSecret(ctx context.Context, key string) (string, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return "", err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	envKey := p.formatEnvKey(key)
	value := os.Getenv(envKey)

	if value == "" && !p.config.AllowMissing {
		return "", ewrap.New("secret not found").
			WithMetadata("key", key)
	}

	return value, nil
}

// SetSecret sets the value of the secret with the given key in the DotEnv provider.
// If the environment variable corresponding to the key cannot be set, an error is returned.
func (p *Provider) SetSecret(_ context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	envKey := p.formatEnvKey(key)

	return ewrap.Wrapf(
		os.Setenv(envKey, value),
		"setting environment variable",
	).WithMetadata("key", envKey)
}

func (p *Provider) formatEnvKey(key string) string {
	if p.config.Prefix == "" {
		return strings.ToUpper(key)
	}

	return fmt.Sprintf("%s_%s", strings.ToUpper(p.config.Prefix), strings.ToUpper(key))
}

func (p *Provider) ensureLoaded(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.loaded {
		return nil
	}

	select {
	case <-ctx.Done():
		return ewrap.Wrap(ctx.Err(), "context canceled while loading secrets")
	default:
		if err := p.loadEnvFile(); err != nil {
			return err
		}

		p.loaded = true

		return nil
	}
}

func (p *Provider) loadEnvFile() error {
	if p.config.Source == secrets.EnvVars {
		return nil
	}

	err := godotenv.Load(p.config.EnvPath)
	if err != nil && p.config.Source == secrets.EnvFile {
		return ewrap.Wrapf(err, "loading env file").
			WithMetadata("path", p.config.EnvPath)
	}

	return nil
}
