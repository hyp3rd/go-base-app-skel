package config

import (
	"strings"
	"time"

	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// implement the validatable interface.
var _ validatable = (*DBConfig)(nil)

// DBConfig holds the SQL databases configuration across the system.
type DBConfig struct {
	DSN             string        `mapstructure:"dsn"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	Host            string        `mapstructure:"host"`
	Port            string        `mapstructure:"port"`
	Database        string        `mapstructure:"database"`
	PoolMode        string        `mapstructure:"pool_mode"`
	MaxOpenConns    int32         `mapstructure:"max_open_conns"`
	MaxIdleConns    int32         `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnAttempts    int           `mapstructure:"conn_attempts"`
	ConnTimeout     time.Duration `mapstructure:"conn_timeout"`
}

func (c *DBConfig) BuildDSN() {
	builder := strings.Builder{}
	builder.WriteString("postgresql://")
	builder.WriteString(c.Username)
	builder.WriteString(":")
	builder.WriteString(c.Password)
	builder.WriteString("@")
	builder.WriteString(c.Host)
	builder.WriteString(":")
	builder.WriteString(c.Port)
	builder.WriteString("/")
	builder.WriteString(c.Database)

	c.DSN = builder.String()
}

// Validate checks the validity of the DBConfig struct and returns an ErrorGroup
// containing any configuration errors found.
func (c *DBConfig) Validate(eg *ewrap.ErrorGroup) {
	if c.DSN == "" {
		eg.Add(ewrap.New("database DSN is required"))
	}

	if c.MaxOpenConns <= 0 {
		eg.Add(ewrap.New("invalid max open connections").WithMetadata("max_open_conns", c.MaxOpenConns))
	}

	if c.MaxIdleConns <= 0 {
		eg.Add(ewrap.New("invalid max idle connections").WithMetadata("max_idle_conns", c.MaxIdleConns))
	}

	if c.ConnMaxLifetime <= 0 {
		eg.Add(ewrap.New("invalid connection max lifetime").WithMetadata("conn_max_lifetime", c.ConnMaxLifetime))
	} else {
		if _, err := time.ParseDuration(c.ConnMaxLifetime.String()); err != nil {
			eg.Add(ewrap.New("invalid connection max lifetime").WithMetadata("conn_max_lifetime", c.ConnMaxLifetime))
		}
	}

	if c.ConnAttempts <= 0 {
		eg.Add(ewrap.New("invalid connection attempts").WithMetadata("conn_attempts", c.ConnAttempts))
	}

	if c.ConnTimeout <= 0 {
		eg.Add(ewrap.New("invalid connection timeout").WithMetadata("conn_timeout", c.ConnTimeout))
	} else {
		if _, err := time.ParseDuration(c.ConnTimeout.String()); err != nil {
			eg.Add(ewrap.New("invalid connection timeout").WithMetadata("conn_timeout", c.ConnTimeout))
		}
	}
}
