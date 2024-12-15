package logger

import (
	"io"
	"os"
	"time"
)

const (
	// DefaultTimeFormat is the default time format for log entries.
	DefaultTimeFormat = time.RFC3339
	// DefaultLevel is the default logging level.
	DefaultLevel = InfoLevel
	// DefaultBufferSize is the default size of the log buffer.
	DefaultBufferSize = 4096
	// DefaultAsyncBufferSize is the default size of the async log buffer.
	DefaultAsyncBufferSize = 1024
)

// Config holds configuration for the logger.
type Config struct {
	// Level is the minimum level to log
	Level Level
	// Output is where the logs will be written
	Output io.Writer
	// EnableStackTrace enables stack trace for error and fatal levels
	EnableStackTrace bool
	// EnableCaller adds the caller information to log entries
	EnableCaller bool
	// TimeFormat specifies the format for timestamps
	TimeFormat string
	// EnableJSON enables JSON output format
	EnableJSON bool
	// BufferSize sets the size of the log buffer
	BufferSize int
	// AsyncBufferSize sets the size of the async log buffer
	AsyncBufferSize int
	// DisableTimestamp disables timestamp in log entries
	DisableTimestamp bool
	// AdditionalFields adds these fields to all log entries
	AdditionalFields []Field
}

// DefaultConfig returns the default logger configuration.
func DefaultConfig() Config {
	return Config{
		// Set a default output destination (os.Stdout)
		Output:           os.Stdout,
		Level:            DefaultLevel,
		EnableStackTrace: true,
		EnableCaller:     true,
		TimeFormat:       DefaultTimeFormat,
		EnableJSON:       false, // Changed to false for better console readability by default
		BufferSize:       DefaultBufferSize,
		AsyncBufferSize:  DefaultAsyncBufferSize,
		AdditionalFields: make([]Field, 0), // Initialize empty slice
	}
}
