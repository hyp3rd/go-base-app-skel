package aws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// Config holds the configuration for the AWS Secrets Manager provider.
type Config struct {
	// Region is the AWS region where secrets are stored.
	Region string
	// BasePath is a prefix added to all secret names.
	BasePath string
	// MaxRetries is the number of retries for failed operations.
	MaxRetries int
	// Timeout for AWS operations.
	Timeout time.Duration
}

// Provider implements the secrets.Provider interface for AWS Secrets Manager.
type Provider struct {
	client     *secretsmanager.Client
	config     Config
	mu         sync.RWMutex
	retryDelay time.Duration
}

// New creates a new AWS Secrets Manager provider instance.
func New(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = constants.DefaultTimeout
	}

	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}

	// Load AWS configuration
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithRetryMaxAttempts(cfg.MaxRetries),
	)
	if err != nil {
		return nil, ewrap.Wrapf(err, "loading AWS config")
	}

	return &Provider{
		client:     secretsmanager.NewFromConfig(awsCfg),
		config:     cfg,
		retryDelay: 1 * time.Second,
	}, nil
}

// GetSecret retrieves a secret from AWS Secrets Manager.
func (p *Provider) GetSecret(ctx context.Context, key string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	secretName := p.buildSecretName(key)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	input := &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	}

	// Get the secret value
	result, err := p.client.GetSecretValue(ctx, input)
	if err != nil {
		return "", ewrap.Wrapf(err, "retrieving secret").
			WithMetadata("key", key)
	}

	// Parse the secret value
	return p.parseSecretValue(result.SecretString, key)
}

// SetSecret stores a secret in AWS Secrets Manager.
func (p *Provider) SetSecret(ctx context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	secretName := p.buildSecretName(key)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// Create the secret value structure
	secretValue := map[string]string{
		"value": value,
	}

	// Convert to JSON
	secretString, err := json.Marshal(secretValue)
	if err != nil {
		return ewrap.Wrapf(err, "marshaling secret value").
			WithMetadata("key", key)
	}

	// Check if the secret already exists
	_, err = p.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	})

	if err == nil {
		// Update existing secret
		input := &secretsmanager.PutSecretValueInput{
			SecretId:     &secretName,
			SecretString: aws.String(string(secretString)),
		}

		_, err = p.client.PutSecretValue(ctx, input)
		if err != nil {
			return ewrap.Wrapf(err, "updating secret").
				WithMetadata("key", key)
		}
	} else {
		// Create new secret
		input := &secretsmanager.CreateSecretInput{
			Name:         &secretName,
			SecretString: aws.String(string(secretString)),
		}

		_, err = p.client.CreateSecret(ctx, input)
		if err != nil {
			return ewrap.Wrapf(err, "creating secret").
				WithMetadata("key", key)
		}
	}

	return nil
}

// buildSecretName constructs the full name for a secret in AWS Secrets Manager.
func (p *Provider) buildSecretName(key string) string {
	if p.config.BasePath == "" {
		return key
	}

	return p.config.BasePath + "/" + key
}

// parseSecretValue extracts the value from a JSON-encoded secret.
func (p *Provider) parseSecretValue(secretString *string, key string) (string, error) {
	if secretString == nil {
		return "", ewrap.New("empty secret value").
			WithMetadata("key", key)
	}

	var secretData map[string]string
	if err := json.Unmarshal([]byte(*secretString), &secretData); err != nil {
		return "", ewrap.Wrapf(err, "parsing secret value").
			WithMetadata("key", key)
	}

	value, ok := secretData["value"]
	if !ok {
		return "", ewrap.New("invalid secret format").
			WithMetadata("key", key)
	}

	return value, nil
}
