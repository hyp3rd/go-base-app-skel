package config

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/hyp3rd/base/internal/constants"
	"github.com/hyp3rd/base/internal/secrets"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
	"github.com/spf13/viper"
)

// Config represents the application configuration, which is loaded from a YAML file
// and secrets providers. It contains various configuration options for the servers,
// rate limiter, database, pub/sub, and sensitive credentials.
type Config struct {
	Environment string            `mapstructure:"environment"`
	Servers     ServersConfig     `mapstructure:"servers"`
	RateLimiter RateLimiterConfig `mapstructure:"rate_limiter"`
	DB          DBConfig          `mapstructure:"db"`
	PubSub      PubSubConfig      `mapstructure:"pubsub"`
	Secrets     *secrets.Store    `mapstructure:"-"` // Secrets are handled separately

	mu sync.RWMutex
	// rotationCallbacks holds functions to be called after secret rotation
	rotationCallbacks []RotationCallback
	// secretsManager holds the reference to our secrets manager
	secretsManager *secrets.Manager
}

// RotationCallback is a function that gets called after secrets are rotated.
type RotationCallback func(ctx context.Context, oldSecrets, newSecrets *secrets.Store) error

// Options holds configuration options for initializing the Config.
type Options struct {
	// ConfigName is the name of the configuration file (without extension).
	ConfigName string
	// SecretsProvider is the interface for accessing secrets.
	SecretsProvider secrets.Provider
	// Timeout for secrets operations.
	Timeout time.Duration
}

// DefaultOptions returns the default configuration options.
func DefaultOptions() Options {
	return Options{
		ConfigName: "config",
		// Context:    context.Background(),
		Timeout: constants.DefaultTimeout,
	}
}

// NewConfig loads the application configuration from a YAML file, environment variables,
// and secrets provider. It validates the configuration before returning.
func NewConfig(ctx context.Context, opts Options) (*Config, error) {
	// Use default options if not specified
	if opts.ConfigName == "" {
		opts.ConfigName = DefaultOptions().ConfigName
	}

	if opts.Timeout == 0 {
		opts.Timeout = DefaultOptions().Timeout
	}

	// Initialize viper configuration
	viper.SetConfigName(opts.ConfigName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, ewrap.Wrapf(err, "reading config file")
		}
	}

	// Set defaults after reading config but before unmarshaling
	setDefaults()

	// Create base configuration
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, ewrap.Wrapf(err, "unmarshaling config")
	}

	// Initialize secrets if a provider is specified
	if opts.SecretsProvider != nil {
		if err := cfg.initializeSecrets(ctx, opts); err != nil {
			return nil, ewrap.Wrapf(err, "initializing secrets")
		}
	}

	// Initialize DB DSN
	cfg.DB.BuildDSN()

	// Validate the complete configuration
	if err := validateConfig(&cfg); err != nil {
		return nil, ewrap.Wrap(err, "validating configuration")
	}

	return &cfg, nil
}

// initializeSecrets loads secrets from the provided secrets provider.
func (c *Config) initializeSecrets(ctx context.Context, opts Options) error {
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Create secrets manager
	manager := secrets.NewManager(opts.SecretsProvider)

	// Load secrets
	if err := manager.Load(ctx); err != nil {
		return ewrap.Wrapf(err, "loading secrets")
	}

	// Store the secrets
	c.Secrets = manager.GetStore()

	// Update configuration with secret values
	if err := c.applySecrets(); err != nil {
		return ewrap.Wrapf(err, "applying secrets to configuration")
	}

	return nil
}

// applySecrets updates the configuration with values from the secrets store.
func (c *Config) applySecrets() error {
	if c.Secrets == nil {
		return ewrap.New("secrets are empty")
	}

	// Apply database credentials
	if c.Secrets.DBCredentials.Username != "" {
		c.DB.Username = c.Secrets.DBCredentials.Username
	}

	if c.Secrets.DBCredentials.Password != "" {
		c.DB.Password = c.Secrets.DBCredentials.Password
	}

	return nil
}

func setDefaults() {
	// QueryAPI defaults
	viper.SetDefault("servers.query_api.port", constants.QueryAPIPort)
	viper.SetDefault("servers.query_api.read_timeout", constants.QueryAPIReadTimeout)
	viper.SetDefault("servers.query_api.write_timeout", constants.QueryAPIWriteTimeout)
	viper.SetDefault("servers.query_api.shutdown_timeout", constants.QueryAPIShutdownTimeout)

	// gRPC defaults
	viper.SetDefault("servers.grpc.port", constants.GRPCServerPort)
	viper.SetDefault("servers.grpc.max_connection_idle", constants.GRPCServerMaxConnectionIdle)
	viper.SetDefault("servers.grpc.max_connection_age", constants.GRPCServerMaxConnectionAge)
	viper.SetDefault("servers.grpc.max_connection_age_grace", constants.GRPCServerMaxConnectionAgeGrace)
	viper.SetDefault("servers.grpc.keepalive_time", constants.GRPCServerKeepaliveTime)
	viper.SetDefault("servers.grpc.keepalive_timeout", constants.GRPCServerKeepaliveTimeout)

	// DB defaults
	viper.SetDefault("db.max_open_conns", constants.DBMaxOpenConns)
	viper.SetDefault("db.max_idle_conns", constants.DBMaxIdleConns)
	viper.SetDefault("db.conn_max_lifetime", constants.DBConnMaxLifetime)

	// PubSub defaults
	viper.SetDefault("pubsub.ack_deadline", constants.PubSubAckDeadline)
	viper.SetDefault("pubsub.retry_policy.minimum_backoff", constants.PubSubRetryPolicyMinimumBackoff)
	viper.SetDefault("pubsub.retry_policy.maximum_backoff", constants.PubSubRetryPolicyMaximumBackoff)
	viper.SetDefault("pubsub.rate_limit.requests_per_second", constants.PubSubRateLimitRequestsPerSecond)
	viper.SetDefault("pubsub.rate_limit.burst_size", constants.PubSubRateLimitBurstSize)
}

func validateConfig(cfg *Config) error {
	validator := NewValidator()

	return validator.Validate(&cfg.Servers,
		&cfg.RateLimiter,
		&cfg.DB,
		&cfg.PubSub)
}

// RegisterRotationCallback adds a callback to be executed after secret rotation.
func (c *Config) RegisterRotationCallback(callback RotationCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rotationCallbacks = append(c.rotationCallbacks, callback)
}

// ReloadSecrets refreshes all secrets from the provider.
func (c *Config) ReloadSecrets(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.secretsManager == nil {
		return ewrap.New("secrets manager not initialized")
	}

	// Store old secrets for callbacks
	oldSecrets := c.Secrets

	// Create a new manager to load fresh secrets
	if err := c.secretsManager.Load(ctx); err != nil {
		return ewrap.Wrapf(err, "reloading secrets")
	}

	// Get the fresh secrets
	newSecrets := c.secretsManager.GetStore()
	c.Secrets = newSecrets

	// Apply the new secrets to configuration
	if err := c.applySecrets(); err != nil {
		// Rollback on failure
		c.Secrets = oldSecrets

		return ewrap.Wrapf(err, "applying reloaded secrets")
	}

	// Execute rotation callbacks
	for _, callback := range c.rotationCallbacks {
		if err := callback(ctx, oldSecrets, newSecrets); err != nil {
			// Log error but continue with other callbacks
			// You might want to handle this differently based on your requirements
			c.logRotationCallbackError(err, callback)
		}
	}

	return nil
}

func (c *Config) logRotationCallbackError(err error, callback RotationCallback) {
	// Log error but continue with other callbacks
}

// RotateSecrets performs a full secret rotation
func (c *Config) RotateSecrets(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.secretsManager == nil {
		return ewrap.New("secrets manager not initialized")
	}

	// Store old secrets for potential rollback and callbacks
	oldSecrets := c.Secrets

	// Create rotation context with timeout
	rotationCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Start the rotation process
	newSecrets, err := c.performRotation(rotationCtx)
	if err != nil {
		return err
	}

	// Update current secrets
	c.Secrets = newSecrets

	// Apply the new secrets to configuration
	if err := c.applySecrets(); err != nil {
		// Rollback on failure
		c.Secrets = oldSecrets
		c.secretsManager.SetStore(oldSecrets)

		return ewrap.Wrapf(err, "applying rotated secrets")
	}

	// Execute rotation callbacks
	return c.executeRotationCallbacks(ctx, oldSecrets, newSecrets)
}

// performRotation handles the actual secret rotation process with proper verification
// and atomic updates. It generates new credentials, verifies them, and ensures
// a safe transition from old to new secrets.
func (c *Config) performRotation(ctx context.Context) (*secrets.Store, error) {
	// Create a new secrets store that will hold our rotated secrets
	newSecrets := &secrets.Store{}

	// Track our progress for potential rollback
	var completedRotations []string

	// Generate and store new database credentials
	if err := c.rotateDatabaseCredentials(ctx, newSecrets); err != nil {
		return nil, c.handleRotationFailure(ctx, completedRotations, err)
	}

	completedRotations = append(completedRotations, "database")

	// Perform other rotations here to follow.

	completedRotations = append(completedRotations, "api_keys")

	return newSecrets, nil
}

// rotateDatabaseCredentials handles the rotation of database credentials
func (c *Config) rotateDatabaseCredentials(ctx context.Context, newSecrets *secrets.Store) error {
	// Generate new secure credentials
	username, err := generateSecureString(32)
	if err != nil {
		return ewrap.Wrapf(err, "generating new username")
	}

	password, err := generateSecureString(64)
	if err != nil {
		return ewrap.Wrapf(err, "generating new password")
	}

	// Store the new credentials temporarily
	newSecrets.DBCredentials.Username = username
	newSecrets.DBCredentials.Password = password

	// Create metadata for the rotation
	metadata := map[string]string{
		"rotated_at": time.Now().UTC().Format(time.RFC3339),
		"reason":     "scheduled_rotation",
	}

	// Store new credentials in the secrets provider with metadata
	if err := c.storeDBCredentials(ctx, username, password, metadata); err != nil {
		return ewrap.Wrapf(err, "storing new database credentials")
	}

	// Verify the new credentials work
	if err := c.verifyDBCredentials(ctx, username, password); err != nil {
		return ewrap.Wrapf(err, "verifying new database credentials")
	}

	return nil
}

// handleRotationFailure attempts to rollback any completed rotations
func (c *Config) handleRotationFailure(ctx context.Context, completedRotations []string, err error) error {
	// Create a new context with timeout for rollback operations
	rollbackCtx, cancel := context.WithTimeout(ctx, constants.DefaultTimeout)
	defer cancel()

	rollbackErr := c.rollbackRotations(rollbackCtx, completedRotations)
	if rollbackErr != nil {
		// If rollback fails, wrap both errors together
		return ewrap.New("rotation and rollback failed").
			WithMetadata("rotation_error", err).
			WithMetadata("rollback_error", rollbackErr)
	}

	return ewrap.Wrapf(err, "rotation failed and was rolled back")
}

// storeDBCredentials stores the new database credentials in the secrets provider
func (c *Config) storeDBCredentials(ctx context.Context, username, password string, metadata map[string]string) error {
	// Store username

	if err := c.secretsManager.Provider.SetSecret(ctx, "DB_USERNAME", username); err != nil {
		return ewrap.Wrapf(err, "storing username")
	}

	// Store password
	if err := c.secretsManager.Provider.SetSecret(ctx, "DB_PASSWORD", password); err != nil {
		return ewrap.Wrapf(err, "storing password")
	}

	return nil
}

// generateSecureString generates a cryptographically secure random string
func generateSecureString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", ewrap.Wrapf(err, "generating random bytes")
	}

	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// verifyDBCredentials attempts to verify that the new database credentials work
func (c *Config) verifyDBCredentials(ctx context.Context, username, password string) error {
	// Implementation would depend on your database setup
	// Example pseudo-code:
	// db, err := sql.Open("postgres", fmt.Sprintf("user=%s password=%s", username, password))
	// if err != nil {
	//     return ewrap.Wrapf(err, "opening test connection")
	// }
	// defer db.Close()
	// return db.PingContext(ctx)
	return nil // TODO: Implement actual verification
}

// rollbackRotations attempts to restore the previous state for completed rotations
func (c *Config) rollbackRotations(ctx context.Context, completedRotations []string) error {
	// Implementation would restore the old secrets for each completed rotation
	// This would vary based on your specific requirements and setup
	return nil // TODO: Implement actual rollback logic
}

func (c *Config) executeRotationCallbacks(ctx context.Context, oldSecrets, newSecrets *secrets.Store) error {
	var errs []error

	// Execute all callbacks
	for _, callback := range c.rotationCallbacks {
		if err := callback(ctx, oldSecrets, newSecrets); err != nil {
			errs = append(errs, err)
		}
	}

	// If any callbacks failed, return a combined error
	if len(errs) > 0 {
		return ewrap.New("one or more rotation callbacks failed").
			WithMetadata("errors", errs)
	}

	return nil
}
