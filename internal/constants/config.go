package constants

import "time"

type ConfigEnvKey string

const (
	EnvPrefix = ConfigEnvKey("BASE")
	// DBUsername is the environment variable name for the database username.
	DBUsername = ConfigEnvKey("DB_USERNAME")
	// DBPassword is the environment variable name for the database password.
	DBPassword = ConfigEnvKey("DB_PASSWORD")
)

// String implements the flag.Value interface.
func (k ConfigEnvKey) String() string {
	return string(k)
}

const (
	DefaultTimeout                   = 30 * time.Second
	QueryAPIPort                     = 8000
	QueryAPIReadTimeout              = "15s"
	QueryAPIWriteTimeout             = "15s"
	QueryAPIShutdownTimeout          = "5s"
	GRPCServerPort                   = 50051
	GRPCServerMaxConnectionIdle      = "15m"
	GRPCServerMaxConnectionAge       = "30m"
	GRPCServerMaxConnectionAgeGrace  = "5m"
	GRPCServerKeepaliveTime          = "5m"
	GRPCServerKeepaliveTimeout       = "20s"
	DBMaxOpenConns                   = 25
	DBMaxIdleConns                   = 25
	DBConnMaxLifetime                = "5m"
	PubSubAckDeadline                = "30s"
	PubSubRetryPolicyMinimumBackoff  = "10s"
	PubSubRetryPolicyMaximumBackoff  = "600s"
	PubSubRateLimitRequestsPerSecond = 100
	PubSubRateLimitBurstSize         = 50
)
