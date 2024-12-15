package vault

import (
	"context"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// Config holds the configuration for the Vault provider.
type Config struct {
	// Address is the URL of the Vault server (e.g., "http://localhost:8200")
	Address string
	// Token is the authentication token for Vault
	Token string
	// MountPath is the path where secrets are mounted (e.g., "secret")
	MountPath string
	// BasePath is the base path under the mount where secrets are stored
	BasePath string
	// Namespace is the Vault Enterprise namespace (optional)
	Namespace string
	// Timeout for Vault operations
	Timeout time.Duration
	// MaxRetries is the number of retries for failed operations
	MaxRetries int
}

// Provider implements the secrets.Provider interface for HashiCorp Vault.
type Provider struct {
	client     *api.Client
	config     Config
	mu         sync.RWMutex
	retryDelay time.Duration
}

// New creates a new Vault provider instance.
func New(cfg Config) (*Provider, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = constants.DefaultTimeout
	}

	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}

	// Create Vault config
	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = cfg.Address

	if cfg.Timeout > 0 {
		vaultConfig.Timeout = cfg.Timeout
	}

	// Create Vault client
	client, err := api.NewClient(vaultConfig)
	if err != nil {
		return nil, ewrap.Wrapf(err, "creating Vault client")
	}

	// Set auth token
	client.SetToken(cfg.Token)

	// Set namespace if provided (Vault Enterprise feature)
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}

	return &Provider{
		client:     client,
		config:     cfg,
		retryDelay: 1 * time.Second,
	}, nil
}

// GetSecret retrieves a secret from Vault with retry logic.
func (p *Provider) GetSecret(ctx context.Context, key string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var (
		secret *api.KVSecret
		err    error
	)

	// Build the full path for the secret
	secretPath := p.buildSecretPath(key)

	// Implement retry logic with exponential backoff
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return "", ewrap.Wrap(ctx.Err(), "context canceled")
		default:
			// Read the secret from Vault
			secret, err = p.client.KVv2(p.config.MountPath).Get(ctx, secretPath)
			if err == nil && secret != nil {
				return p.extractSecretValue(secret, key)
			}

			// Check if we should retry
			if attempt == p.config.MaxRetries {
				return "", ewrap.Wrapf(err, "failed to retrieve secret after %d attempts", attempt+1).
					WithMetadata("path", secretPath)
			}

			// Wait before retrying with exponential backoff
			time.Sleep(p.retryDelay * time.Duration(1<<attempt))
		}
	}

	return "", ewrap.New("unexpected error in retry loop").
		WithMetadata("path", secretPath)
}

// SetSecret stores a secret in Vault with retry logic.
func (p *Provider) SetSecret(ctx context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	secretPath := p.buildSecretPath(key)
	data := map[string]interface{}{
		"value": value,
	}

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ewrap.Wrap(ctx.Err(), "context canceled")
		default:
			// Write the secret to Vault
			_, err := p.client.KVv2(p.config.MountPath).Put(ctx, secretPath, data)
			if err == nil {
				return nil
			}

			// Check if we should retry
			if attempt == p.config.MaxRetries {
				return ewrap.Wrapf(err, "failed to store secret after %d attempts", attempt+1).
					WithMetadata("path", secretPath)
			}

			// Wait before retrying with exponential backoff
			time.Sleep(p.retryDelay * time.Duration(1<<attempt))
		}
	}

	return ewrap.New("unexpected error in retry loop").
		WithMetadata("path", secretPath)
}

// buildSecretPath constructs the full path for a secret in Vault.
func (p *Provider) buildSecretPath(key string) string {
	// Clean and normalize the path components
	// mountPath := strings.Trim(p.config.MountPath, "/")
	basePath := strings.Trim(p.config.BasePath, "/")
	key = strings.Trim(key, "/")

	// Combine the path components
	return path.Join(basePath, key)
}

// extractSecretValue retrieves the value from a Vault secret.
func (p *Provider) extractSecretValue(secret *api.KVSecret, key string) (string, error) {
	if secret.Data == nil {
		return "", ewrap.New("empty secret data").
			WithMetadata("key", key)
	}

	// KVSecret already contains the decrypted data directly
	value, ok := secret.Data["value"].(string)
	if !ok {
		return "", ewrap.New("secret value is not a string").
			WithMetadata("key", key)
	}

	return value, nil
}

// Health checks the health status of the Vault server.
func (p *Provider) Health(_ context.Context) error {
	health, err := p.client.Sys().Health()
	if err != nil {
		return ewrap.Wrapf(err, "checking Vault health")
	}

	if !health.Initialized {
		return ewrap.New("Vault is not initialized")
	}

	if health.Sealed {
		return ewrap.New("Vault is sealed")
	}

	return nil
}
