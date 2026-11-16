package krillion

// parse (stage 1) then validate (stage 2).
// errors are ErrNotKrillion, *ParseError, or one or more *ValidationError
// joined with errors.Join (one reason per line in err.Error())
func FromMessage(content string) (PlayerScore, error) {
	p, err := Parse(content)
	if err != nil {
		return PlayerScore{}, err
	}
	if err := Validate(p); err != nil {
		return PlayerScore{}, err
	}
	return p, nil
}
