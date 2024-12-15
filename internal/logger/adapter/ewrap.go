package adapter

import (
	"os"

	"github.com/hyp3rd/ewrap/pkg/ewrap/adapters"
	"github.com/rs/zerolog"
)

// NewZerologAdapter creates a new ZerologAdapter instance that wraps the zerolog logger.
// The zerolog logger is configured to write to stdout and include timestamps.
func NewZerologAdapter() *adapters.ZerologAdapter {
	zerologLogger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	return adapters.NewZerologAdapter(zerologLogger)
}
