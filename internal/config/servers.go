package config

import (
	"time"

	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// implement the validatable interface.
var _ validatable = (*ServersConfig)(nil)

// ServersConfig holds the servers configuration across the system.
type ServersConfig struct {
	QueryAPI QueryAPIConfig `mapstructure:"query_api"`
	GRPC     GRPCConfig     `mapstructure:"grpc"`
}

// QueryServerConfig holds the Query API http server configuration.
type QueryAPIConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// GRPCConfig holds the gRPC servers configuration.
type GRPCConfig struct {
	Port                  int           `mapstructure:"port"`
	MaxConnectionIdle     time.Duration `mapstructure:"max_connection_idle"`
	MaxConnectionAge      time.Duration `mapstructure:"max_connection_age"`
	MaxConnectionAgeGrace time.Duration `mapstructure:"max_connection_age_grace"`
	KeepAliveTime         time.Duration `mapstructure:"keepalive_time"`
	KeepAliveTimeout      time.Duration `mapstructure:"keepalive_timeout"`
}

// Validate validates the ServersConfig by checking the validity of the QueryAPI and GRPC configurations.
func (c *ServersConfig) Validate(eg *ewrap.ErrorGroup) {
	c.validateQueryAPI(eg)
	c.validateGRPC(eg)
}

func validPort(port int, privileged bool) bool {
	// ensure the port is valid and in the range 1-65535
	if privileged {
		return port > 0 && port <= 65535
	}

	return port > 1023 && port <= 65535
}

func (c *ServersConfig) validateQueryAPI(eg *ewrap.ErrorGroup) {
	if !validPort(c.QueryAPI.Port, false) {
		eg.Add(ewrap.New("query API port must be greater than 1023 and less than 65535"))
	}

	if c.QueryAPI.ReadTimeout <= 0 {
		eg.Add(ewrap.New("query API read timeout must be greater than 0"))
	} else if _, err := time.ParseDuration(c.QueryAPI.ReadTimeout.String()); err != nil {
		eg.Add(ewrap.Wrap(err, "query API read timeout is invalid"))
	}

	if c.QueryAPI.WriteTimeout <= 0 {
		eg.Add(ewrap.New("query API write timeout must be greater than 0"))
	} else if _, err := time.ParseDuration(c.QueryAPI.WriteTimeout.String()); err != nil {
		eg.Add(ewrap.Wrap(err, "query API write timeout is invalid"))
	}

	if c.QueryAPI.ShutdownTimeout <= 0 {
		eg.Add(ewrap.New("query API shutdown timeout must be greater than 0"))
	} else if _, err := time.ParseDuration(c.QueryAPI.ShutdownTimeout.String()); err != nil {
		eg.Add(ewrap.Wrap(err, "query API shutdown timeout is invalid"))
	}
}

func (c *ServersConfig) validateGRPC(eg *ewrap.ErrorGroup) {
	if !validPort(c.QueryAPI.Port, false) {
		eg.Add(ewrap.New("gRPC port must be greater than 0"))
	}

	if c.GRPC.MaxConnectionIdle <= 0 {
		eg.Add(ewrap.New("gRPC max connection idle must be greater than 0"))
	}

	if c.GRPC.MaxConnectionAge <= 0 {
		eg.Add(ewrap.New("gRPC max connection age must be greater than 0"))
	}

	if c.GRPC.MaxConnectionAgeGrace <= 0 {
		eg.Add(ewrap.New("gRPC max connection age grace must be greater than 0"))
	} else if _, err := time.ParseDuration(c.GRPC.MaxConnectionAgeGrace.String()); err != nil {
		eg.Add(ewrap.Wrap(err, "gRPC max connection age grace is invalid"))
	}

	if c.GRPC.KeepAliveTime <= 0 {
		eg.Add(ewrap.New("gRPC keepalive time must be greater than 0"))
	} else if _, err := time.ParseDuration(c.GRPC.KeepAliveTime.String()); err != nil {
		eg.Add(ewrap.Wrap(err, "gRPC keepalive time is invalid"))
	}

	if c.GRPC.KeepAliveTimeout <= 0 {
		eg.Add(ewrap.New("gRPC keepalive timeout must be greater than 0"))
	} else if _, err := time.ParseDuration(c.GRPC.KeepAliveTimeout.String()); err != nil {
		eg.Add(ewrap.Wrap(err, "gRPC keepalive timeout is invalid"))
	}
}
