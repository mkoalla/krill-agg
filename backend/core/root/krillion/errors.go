package krillion

import "errors"

// the message has no Krillion header at all; callers should ignore it silently
var ErrNotKrillion = errors.New("not a krillion message")

// stage 1: the message looked like a Krillion result but couldn't be read
type ParseError struct{ Reason string }

func (e *ParseError) Error() string { return e.Reason }

// stage 2: the message was read but the result doesn't add up
type ValidationError struct{ Reason string }

func (e *ValidationError) Error() string { return e.Reason }
