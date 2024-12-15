package config

import (
	"time"

	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

// implement the validatable interface.
var _ validatable = (*PubSubConfig)(nil)

// PubSubConfig holds the pubsub (typically GCP) configuration, globally for the system.
type PubSubConfig struct {
	ProjectID      string        `mapstructure:"project_id"`
	TopicID        string        `mapstructure:"topic_id"`
	SubscriptionID string        `mapstructure:"subscription_id"`
	EmulatorHost   string        `mapstructure:"emulator_host"`
	AckDeadline    time.Duration `mapstructure:"ack_deadline"`
	Subscription   Subscription  `mapstructure:"subscription"`
	RetryPolicy    RetryPolicy   `mapstructure:"retry_policy"`
}

type Subscription struct {
	ReceiveMaxOutstandingMessages int           `mapstructure:"receive_max_outstanding_messages"`
	ReceiveNumGoroutines          int           `mapstructure:"receive_num_goroutines"`
	ReceiveMaxExtension           time.Duration `mapstructure:"receive_max_extension"`
}

// RetryPolicy holds the retry policy for pubsub messages.
type RetryPolicy struct {
	MaxAttempts    int           `mapstructure:"max_attempts"`
	MinimumBackoff time.Duration `mapstructure:"minimum_backoff"`
	MaximumBackoff time.Duration `mapstructure:"maximum_backoff"`
}

// Validate checks the validity of the PubSubConfig and returns an ErrorGroup containing any
// configuration errors. It ensures that either project_id or emulator_host is set, and that
// topic_id and subscription_id are not empty. It also validates the ack_deadline and
// retry_policy configurations.
func (c *PubSubConfig) Validate(eg *ewrap.ErrorGroup) {
	if c.ProjectID == "" && c.EmulatorHost == "" {
		eg.Add(ewrap.New("either project_id or emulator_host is required for PubSub"))
	}

	if c.TopicID == "" {
		eg.Add(ewrap.New("topic_id is required for PubSub"))
	}

	if c.SubscriptionID == "" {
		eg.Add(ewrap.New("subscription_id is required for PubSub"))
	}

	c.validateAckDeadline(eg)
	c.validateSubscription(eg)
	c.validateRetryPolicy(eg)
}

func (c *PubSubConfig) validateAckDeadline(eg *ewrap.ErrorGroup) {
	if c.AckDeadline <= 0 {
		eg.Add(ewrap.New("invalid pubsub ack_deadline").WithMetadata("ack_deadline", c.AckDeadline))
	} else if _, err := time.ParseDuration(c.AckDeadline.String()); err != nil {
		eg.Add(ewrap.New("invalid pubsub ack_deadline").WithMetadata("ack_deadline", c.AckDeadline))
	}
}

func (c *PubSubConfig) validateSubscription(eg *ewrap.ErrorGroup) {
	if c.Subscription.ReceiveMaxOutstandingMessages <= 0 {
		eg.Add(ewrap.New("invalid pubsub subscription receive_max_outstanding_messages").WithMetadata("receive_max_outstanding_messages", c.Subscription.ReceiveMaxOutstandingMessages))
	}

	if c.Subscription.ReceiveNumGoroutines <= 0 {
		eg.Add(ewrap.New("invalid pubsub subscription receive_num_goroutines").WithMetadata("receive_num_goroutines", c.Subscription.ReceiveNumGoroutines))
	}

	if c.Subscription.ReceiveMaxExtension <= 0 {
		eg.Add(ewrap.New("invalid pubsub subscription receive_max_extension").WithMetadata("receive_max_extension", c.Subscription.ReceiveMaxExtension))
	} else if _, err := time.ParseDuration(c.Subscription.ReceiveMaxExtension.String()); err != nil {
		eg.Add(ewrap.New("invalid pubsub subscription receive_max_extension").WithMetadata("receive_max_extension", c.Subscription.ReceiveMaxExtension))
	}
}

func (c *PubSubConfig) validateRetryPolicy(eg *ewrap.ErrorGroup) {
	if c.RetryPolicy.MaxAttempts <= 0 || c.RetryPolicy.MaxAttempts > 10 {
		eg.Add(ewrap.New("invalid pubsub retry_policy max_attempts").WithMetadata("max_attempts", c.RetryPolicy.MaxAttempts))
	}

	if c.RetryPolicy.MinimumBackoff <= 0 {
		eg.Add(ewrap.New("invalid pubsub retry_policy minimum_backoff").WithMetadata("minimum_backoff", c.RetryPolicy.MinimumBackoff))
	} else if _, err := time.ParseDuration(c.RetryPolicy.MinimumBackoff.String()); err != nil {
		eg.Add(ewrap.New("invalid pubsub retry_policy minimum_backoff").WithMetadata("minimum_backoff", c.RetryPolicy.MinimumBackoff))
	}

	if c.RetryPolicy.MaximumBackoff <= 0 {
		eg.Add(ewrap.New("invalid pubsub retry_policy maximum_backoff").WithMetadata("maximum_backoff", c.RetryPolicy.MaximumBackoff))
	} else if _, err := time.ParseDuration(c.RetryPolicy.MaximumBackoff.String()); err != nil {
		eg.Add(ewrap.New("invalid pubsub retry_policy maximum_backoff").WithMetadata("maximum_backoff", c.RetryPolicy.MaximumBackoff))
	}
}
