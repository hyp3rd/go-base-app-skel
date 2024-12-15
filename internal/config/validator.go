package config

import (
	"errors"

	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

type validatable interface {
	Validate(eg *ewrap.ErrorGroup)
}

// Validator is a struct that holds an ErrorGroup for collecting validation errors.
type Validator struct {
	Errors *ewrap.ErrorGroup
}

// NewValidator creates a new Validator instance with an empty ErrorGroup.
func NewValidator() *Validator {
	return &Validator{
		Errors: ewrap.NewErrorGroup(),
	}
}

// Validate validates the given validatable configurations and returns an error if any of them are invalid.
// The Validator collects all errors in its Errors field, which can be inspected after calling Validate.
func (v *Validator) Validate(configs ...validatable) error {
	for _, c := range configs {
		c.Validate(v.Errors)
	}

	if v.Errors.HasErrors() {
		return errors.Join(v.Errors.Errors()...)
	}

	return nil
}
