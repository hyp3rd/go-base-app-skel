package azure

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// Config holds the configuration for the Azure Key Vault provider.
type Config struct {
	// VaultName is the name of the Azure Key Vault.
	VaultName string
	// TenantID is the Azure AD tenant ID.
	TenantID string
	// ClientID is the Azure AD application (client) ID.
	ClientID string
	// ClientSecret is the Azure AD application client secret.
	ClientSecret string
	// UseManagedIdentity indicates whether to use Azure Managed Identity.
	UseManagedIdentity bool
	// Timeout for Azure operations.
	Timeout time.Duration
	// MaxRetries is the number of retries for failed operations.
	MaxRetries int
	// Tags to apply to secrets (key-value pairs).
	Tags map[string]*string
}

// Provider implements the secrets.Provider interface for Azure Key Vault.
type Provider struct {
	client     *azsecrets.Client
	config     Config
	mu         sync.RWMutex
	retryDelay time.Duration
}

// New creates a new Azure Key Vault provider instance.
func New(_ context.Context, cfg Config) (*Provider, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = constants.DefaultTimeout
	}

	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}

	var (
		cred azcore.TokenCredential
		err  error
	)

	if cfg.UseManagedIdentity {
		// Use managed identity authentication
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	} else {
		// Use service principal authentication
		cred, err = azidentity.NewClientSecretCredential(
			cfg.TenantID,
			cfg.ClientID,
			cfg.ClientSecret,
			nil,
		)
	}

	if err != nil {
		return nil, ewrap.Wrapf(err, "creating Azure credentials")
	}

	// Create Key Vault client
	vaultURL := fmt.Sprintf("https://%s.vault.azure.net/", cfg.VaultName)

	client, err := azsecrets.NewClient(vaultURL, cred, nil)
	if err != nil {
		return nil, ewrap.Wrapf(err, "creating Key Vault client")
	}

	return &Provider{
		client:     client,
		config:     cfg,
		retryDelay: 1 * time.Second,
	}, nil
}

// GetSecret retrieves a secret from Azure Key Vault.
func (p *Provider) GetSecret(ctx context.Context, key string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	var (
		resp azsecrets.GetSecretResponse
		err  error
	)

	// Implement retry logic with exponential backoff
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		resp, err = p.client.GetSecret(ctx, key, "", nil)
		if err == nil {
			return *resp.Value, nil
		}

		if attempt == p.config.MaxRetries {
			return "", ewrap.Wrapf(err, "retrieving secret").
				WithMetadata("key", key).
				WithMetadata("attempt", attempt+1)
		}

		time.Sleep(p.retryDelay * time.Duration(1<<attempt))
	}

	return "", ewrap.New("unexpected error in retry loop").
		WithMetadata("key", key)
}

// SetSecret stores a secret in Azure Key Vault.
func (p *Provider) SetSecret(ctx context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	params := azsecrets.SetSecretParameters{
		Value: to.Ptr(value),
		Tags:  p.config.Tags,
	}

	_, err := p.client.SetSecret(ctx, key, params, nil)
	if err != nil {
		return ewrap.Wrapf(err, "setting secret").
			WithMetadata("key", key)
	}

	return nil
}

// DeleteSecret deletes a secret from Azure Key Vault.
func (p *Provider) DeleteSecret(ctx context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	_, err := p.client.DeleteSecret(ctx, key, nil)
	if err != nil {
		return ewrap.Wrapf(err, "deleting secret").
			WithMetadata("key", key)
	}

	return nil
}

// ListSecrets lists all secrets in the vault.
func (p *Provider) ListSecrets(ctx context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	pager := p.client.NewListSecretPropertiesPager(nil)

	var secrets []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, ewrap.Wrapf(err, "listing secrets")
		}

		for _, item := range page.Value {
			if item.ID != nil {
				// Extract the secret name from the full URL
				secretName := extractSecretNameFromID(string(*item.ID))
				if secretName != "" {
					secrets = append(secrets, secretName)
				}
			}
		}
	}

	return secrets, nil
}

// extractSecretNameFromID extracts the secret name from a fully qualified Azure Key Vault secret ID.
// Example input: "https://my-vault.vault.azure.net/secrets/my-secret-name/version"
// Returns: "my-secret-name".
func extractSecretNameFromID(id string) string {
	// Split the URL path into components
	parts := strings.Split(id, "/secrets/")
	//nolint:mnd
	if len(parts) != 2 {
		return ""
	}

	// Get the secret name and remove any version information
	secretNameWithVersion := parts[1]

	return path.Base(secretNameWithVersion)
}
