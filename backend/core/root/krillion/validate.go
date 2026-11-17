package krillion

import (
	"errors"
	"fmt"
)

// stage 2: independent checks on an already-parsed result, combined with errors.Join
func Validate(p PlayerScore) error {
	return errors.Join(validateResult(p), validateDay(p))
}

// the checks on the result itself, which a stored row (see Rebuild) must
// still pass. its day was checked when it came in
func validateResult(p PlayerScore) error {
	var errs []error
	for _, check := range []func(PlayerScore) error{validateCount, validateTypes, validateSum} {
		if err := check(p); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// the puzzle that's live now; a var so tests can move the clock
var today = Today

// the puzzle must already be out: nobody has played tomorrow's
func validateDay(p PlayerScore) error {
	if now := today(); p.Day > now {
		return &ValidationError{fmt.Sprintf("krillion #%d hasn't happened yet (today is #%d)", p.Day, now)}
	}
	return nil
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
