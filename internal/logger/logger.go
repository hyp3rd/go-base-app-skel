package logger

import (
	"context"
)

// Level represents the severity of a log message.
type Level uint8

const (
	// TraceLevel represents verbose debugging information.
	TraceLevel Level = iota
	// DebugLevel represents debugging information.
	DebugLevel
	// InfoLevel represents general operational information.
	InfoLevel
	// WarnLevel represents warning messages.
	WarnLevel
	// ErrorLevel represents error messages.
	ErrorLevel
	// FatalLevel represents fatal error messages.
	FatalLevel
)

// String returns the string representation of a log level.
func (l Level) String() string {
	switch l {
	case TraceLevel:
		return "TRACE"
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Field represents a key-value pair in structured logging.
type Field struct {
	Key   string
	Value interface{}
}

// Logger defines the interface for logging operations.
type Logger interface {
	// Log methods for different levels
	Trace(msg string)
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Fatal(msg string)

	// Formatted log methods
	FormattedLogger

	Methods
}

// Methods defines the interface for logging methods.
type Methods interface {
	// WithContext adds context information to the logger
	WithContext(ctx context.Context) Logger
	// WithFields adds structured fields to the logger
	WithFields(fields ...Field) Logger
	// WithError adds an error to the logger
	WithError(err error) Logger
	// GetLevel returns the current logging level
	GetLevel() Level
	// SetLevel sets the logging level
	SetLevel(level Level)
	// Sync ensures all logs are written
	Sync() error
}

// FormattedLogger defines the interface for logging formatted messages.
type FormattedLogger interface {
	// Tracef logs a message at the Trace level
	Tracef(format string, args ...interface{})
	// Debugf logs a message at the Debug level
	Debugf(format string, args ...interface{})
	// Infof logs a message at the Info level
	Infof(format string, args ...interface{})
	// Warnf logs a message at the Warn level
	Warnf(format string, args ...interface{})
	// Errorf logs a message at the Error level
	Errorf(format string, args ...interface{})
	// Fatalf logs a message at the Fatal level
	Fatalf(format string, args ...interface{})
}
