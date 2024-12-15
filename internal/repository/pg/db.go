package pg

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hyp3rd/base/internal/config"
	"github.com/hyp3rd/base/internal/logger"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Manager is a struct that manages the connection to a PostgreSQL database.
// It holds a connection pool, the database configuration, and a logger.
type Manager struct {
	pool   *pgxpool.Pool
	cfg    *config.DBConfig
	logger logger.Logger
}

// New creates a new instance of the Manager struct, which manages the connection
// to a PostgreSQL database. It takes a DBConfig and a Logger as arguments, and
// initializes the cfg and logger fields of the Manager.
func New(cfg *config.DBConfig, logger logger.Logger) *Manager {
	return &Manager{
		cfg:    cfg,
		logger: logger,
	}
}

// Connect establishes a connection to the PostgreSQL database using the configuration
// provided in the Manager. It attempts to connect with retries, and verifies the
// connection before returning. If the connection cannot be established after the
// configured number of attempts, an error is returned.
func (m *Manager) Connect(ctx context.Context) error {
	var err error

	// Configure the connection pool
	poolConfig, err := pgxpool.ParseConfig(m.cfg.DSN)
	if err != nil {
		return ewrap.Wrapf(err, "parsing database config")
	}

	// Apply configuration
	poolConfig.MaxConns = m.cfg.MaxOpenConns
	poolConfig.MinConns = m.cfg.MaxIdleConns
	poolConfig.MaxConnLifetime = m.cfg.ConnMaxLifetime

	// Attempt to connect with retries
	for attempt := 1; attempt <= m.cfg.ConnAttempts; attempt++ {
		// Create a context with timeout for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, m.cfg.ConnTimeout)

		m.pool, err = pgxpool.NewWithConfig(attemptCtx, poolConfig)

		cancel()

		if err == nil {
			break
		}

		if attempt == m.cfg.ConnAttempts {
			return ewrap.Wrapf(err, "failed to connect to database after %d attempts", attempt).
				WithMetadata("dsn", maskDSN(m.cfg.DSN))
		}

		m.logger.Warnf("Database connection attempt %d/%d failed: %v",
			attempt, m.cfg.ConnAttempts, err)

		select {
		case <-ctx.Done():
			return ewrap.Wrap(ctx.Err(), "context cancelled during connection attempts")
		case <-time.After(time.Second * time.Duration(attempt)):
			// Exponential backoff
			continue
		}
	}

	// Verify the connection
	if err := m.Ping(ctx); err != nil {
		return ewrap.Wrapf(err, "verifying database connection")
	}

	return nil
}

// Ping checks if the database connection is active by pinging the database.
// If the connection is not established or the ping fails, it returns an error.
func (m *Manager) Ping(ctx context.Context) error {
	if m.pool == nil {
		return ewrap.New("database not connected")
	}

	// Create a context with timeout for this attempt
	attemptCtx, cancel := context.WithTimeout(ctx, m.cfg.ConnTimeout)
	defer cancel()

	err := m.pool.Ping(attemptCtx)
	if err != nil {
		return ewrap.Wrapf(err, "pinging database")
	}

	return nil
}

// Close closes the database connection.
func (m *Manager) Close() {
	if m.pool != nil {
		m.pool.Close()
	}
}

// GetPool returns the connection pool.
func (m *Manager) GetPool() *pgxpool.Pool {
	return m.pool
}

// Stats returns the current pool statistics. If the connection pool is not
// established, it returns nil. If the pool.Stat() method returns nil, it
// returns a new pgxpool.Stat instance.
func (m *Manager) Stats() *pgxpool.Stat {
	if m.pool == nil {
		return nil
	}

	// Return the current pool statistics
	if m.pool.Stat() == nil {
		return &pgxpool.Stat{}
	}

	return m.pool.Stat()
}

// IsConnected checks if the database connection is active. It verifies the connection
// by calling the Ping method. If the connection is not established or the Ping
// fails, it returns false.
func (m *Manager) IsConnected(ctx context.Context) bool {
	if m.pool == nil {
		return false
	}

	// Verify the connection
	if err := m.Ping(ctx); err != nil {
		m.logger.Warnf("Database connection failed: %v", err)

		return false
	}

	return true
}

// Transaction executes the provided function within a database transaction. If the
// function returns an error, the transaction is rolled back. Otherwise, the
// transaction is committed.
//
// The provided function is passed the current context and a pgx.Tx instance to
// execute database operations within the transaction.
//
// If the database connection is not established, an error is returned.
func (m *Manager) Transaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if m.pool == nil {
		return ewrap.New("database not connected")
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return ewrap.Wrapf(err, "beginning transaction")
	}

	// Execute the provided function
	if err := fn(ctx, tx); err != nil {
		// Attempt to rollback on error
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return ewrap.New("transaction failed").
				WithMetadata("exec_error", err).
				WithMetadata("rollback_error", rbErr)
		}

		return ewrap.Wrapf(err, "executing transaction")
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return ewrap.Wrapf(err, "committing transaction")
	}

	return nil
}

// maskDSN takes a database connection string (DSN) and returns a masked version
// of the DSN, hiding sensitive information like the password.
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "[INVALID_DSN]"
	}

	masked := buildMaskedDSN(config)

	return masked
}

func buildMaskedDSN(config *pgx.ConnConfig) string {
	masked := "postgres://"

	if config.User != "" {
		masked += config.User
	}

	if config.Password != "" {
		masked += ":********"
	}

	if config.Host != "" {
		masked += "@" + config.Host
		if config.Port != 0 {
			masked += ":" + strconv.Itoa(int(config.Port))
		}
	}

	if config.Database != "" {
		masked += "/" + config.Database
	}

	masked += addRuntimeParams(config.RuntimeParams)

	return masked
}

func addRuntimeParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	var paramStrings []string

	for key, value := range params {
		if isSensitiveParam(key) {
			paramStrings = append(paramStrings, key+"=[MASKED]")
		} else {
			paramStrings = append(paramStrings, key+"="+value)
		}
	}

	return "?" + strings.Join(paramStrings, "&")
}

// isSensitiveParam checks if a connection parameter is sensitive.
func isSensitiveParam(param string) bool {
	sensitiveParams := map[string]bool{
		"password":    true,
		"sslkey":      true,
		"sslcert":     true,
		"sslrootcert": true,
		"sslpassword": true,
	}

	return sensitiveParams[param]
}
