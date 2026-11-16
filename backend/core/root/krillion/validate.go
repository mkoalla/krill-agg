package krillion

import (
	"errors"
	"fmt"
)

// stage 2: independent checks on an already-parsed result, combined with errors.Join
func Validate(p PlayerScore) error {
	var errs []error
	if err := validateCount(p); err != nil {
		errs = append(errs, err)
	}
	if err := validateTypes(p); err != nil {
		errs = append(errs, err)
	}
	if err := validateSum(p); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// the chain must be exactly NumChoices long
func validateCount(p PlayerScore) error {
	if len(p.Choices) == NumChoices {
		return nil
	}
	return &ValidationError{fmt.Sprintf("invalid num. scores (%d != %d)", len(p.Choices), NumChoices)}
}

// the chain was cut short by an unrecognized emoji glued right onto it
func validateTypes(p PlayerScore) error {
	if p.stoppedAt == "" {
		return nil
	}
	return &ValidationError{fmt.Sprintf("unknown score type: %s", p.stoppedAt)}
}

// the claimed score must equal the sum of the emoji values
func validateSum(p PlayerScore) error {
	sum := 0
	for _, c := range p.Choices {
		sum += ValueOf(c)
	}
	if sum == p.Score {
		return nil
	}
	return &ValidationError{fmt.Sprintf("score mismatched to emojis (claimed %d, counted %d)", p.Score, sum)}
}
