package gcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config holds the configuration for the GCP Secret Manager provider.
type Config struct {
	// ProjectID is the Google Cloud project ID.
	ProjectID string
	// CredentialsFile is the path to the service account JSON file
	// If empty, uses Application Default Credentials.
	CredentialsFile string
	// BasePath is a prefix added to all secret names.
	BasePath string
	// Timeout for GCP operations
	Timeout time.Duration
	// MaxRetries is the number of retries for failed operations.
	MaxRetries int
	// Labels to apply to secrets (key-value pairs).
	Labels map[string]string
}

// Provider implements the secrets.Provider interface for Google Cloud Secret Manager.
type Provider struct {
	client     *secretmanager.Client
	config     Config
	mu         sync.RWMutex
	retryDelay time.Duration
}

// New creates a new GCP Secret Manager provider instance.
func New(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = constants.DefaultTimeout
	}

	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}

	var opts []option.ClientOption
	if cfg.CredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.CredentialsFile))
	}

	// Create Secret Manager client
	client, err := secretmanager.NewClient(ctx, opts...)
	if err != nil {
		return nil, ewrap.Wrapf(err, "creating Secret Manager client")
	}

	return &Provider{
		client:     client,
		config:     cfg,
		retryDelay: 1 * time.Second,
	}, nil
}

// GetSecret retrieves a secret from GCP Secret Manager.
func (p *Provider) GetSecret(ctx context.Context, key string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	secretName := p.buildSecretName(key)

	// Access the latest version of the secret
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: secretName + "/versions/latest",
	}

	var (
		result *secretmanagerpb.AccessSecretVersionResponse
		err    error
	)

	// Implement retry logic with exponential backoff
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		result, err = p.client.AccessSecretVersion(ctx, req)
		if err == nil {
			return string(result.GetPayload().GetData()), nil
		}

		if attempt == p.config.MaxRetries {
			return "", ewrap.Wrapf(err, "accessing secret version").
				WithMetadata("key", key).
				WithMetadata("attempt", attempt+1)
		}

		time.Sleep(p.retryDelay * time.Duration(1<<attempt))
	}

	return "", nil
}

// SetSecret stores a secret in GCP Secret Manager.
func (p *Provider) SetSecret(ctx context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	secretName := p.buildSecretName(key)

	// Check if the secret exists
	secretExists, err := p.secretExists(ctx, secretName)
	if err != nil {
		return err
	}

	if !secretExists {
		// Create the secret
		createReq := &secretmanagerpb.CreateSecretRequest{
			Parent:   "projects/" + p.config.ProjectID,
			SecretId: key,
			Secret: &secretmanagerpb.Secret{
				Labels: p.config.Labels,
				Replication: &secretmanagerpb.Replication{
					Replication: &secretmanagerpb.Replication_Automatic_{
						Automatic: &secretmanagerpb.Replication_Automatic{},
					},
				},
			},
		}

		if _, err := p.client.CreateSecret(ctx, createReq); err != nil {
			return ewrap.Wrapf(err, "creating secret").
				WithMetadata("key", key)
		}
	}

	// Add new version
	addReq := &secretmanagerpb.AddSecretVersionRequest{
		Parent: secretName,
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	}

	_, err = p.client.AddSecretVersion(ctx, addReq)
	if err != nil {
		return ewrap.Wrapf(err, "adding secret version").
			WithMetadata("key", key)
	}

	return nil
}

// secretExists checks if a secret already exists.
func (p *Provider) secretExists(ctx context.Context, name string) (bool, error) {
	req := &secretmanagerpb.GetSecretRequest{
		Name: name,
	}

	_, err := p.client.GetSecret(ctx, req)
	if err != nil {
		// Check if the error is "not found"
		if isNotFoundError(err) {
			return false, nil
		}

		return false, ewrap.Wrapf(err, "checking secret existence")
	}

	return true, nil
}

// buildSecretName constructs the full name for a secret in GCP Secret Manager.
func (p *Provider) buildSecretName(key string) string {
	if p.config.BasePath != "" {
		key = fmt.Sprintf("%s/%s", p.config.BasePath, key)
	}

	return fmt.Sprintf("projects/%s/secrets/%s", p.config.ProjectID, key)
}

// Close closes the GCP client connection.
func (p *Provider) Close() error {
	err := p.client.Close()
	if err != nil {
		return ewrap.Wrapf(err, "closing client")
	}

	return nil
}

// isNotFoundError checks if the provided error is a GCP "not found" error.
// It properly handles the error type conversion and status code checking
// according to Google Cloud API conventions.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Convert the error to a gRPC status
	st, ok := status.FromError(err)
	if !ok {
		// If the error cannot be converted to a status, it's not a GCP API error
		return false
	}

	// Check if the status code matches codes.NotFound (5)
	// This is the standard way GCP indicates a resource doesn't exist
	return st.Code() == codes.NotFound
}
