package config

import "github.com/hyp3rd/ewrap/pkg/ewrap"

// implement the validatable interface.
var _ validatable = (*RateLimiterConfig)(nil)

// RateLimiterConfig holds the rate limiter configuration, globally for the system.
type RateLimiterConfig struct {
	RequestsPerSecond int `mapstructure:"requests_per_second"`
	BurstSize         int `mapstructure:"burst_size"`
}

// Validate ensures the RateLimiterConfig is valid. It checks that the requests_per_second and burst_size
// values are greater than 0, and that requests_per_second is greater than burst_size.
// If any of these conditions are not met, it adds an error to the provided ErrorGroup.
func (c *RateLimiterConfig) Validate(eg *ewrap.ErrorGroup) {
	if c.RequestsPerSecond <= 0 {
		eg.Add(ewrap.New("rate limiter requests_per_second must be greater than 0"))
	}

	if c.BurstSize <= 0 {
		eg.Add(ewrap.New("rate limiter burst_size must be greater than 0"))
	}

	if c.RequestsPerSecond < c.BurstSize {
		eg.Add(ewrap.New("rate limiter requests_per_second must be greater than burst_size"))
	}
}
