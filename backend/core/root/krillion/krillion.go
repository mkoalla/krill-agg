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

// rebuilds a stored result from its emoji string, recomputing the score
func Rebuild(day int, emojis string) (PlayerScore, error) {
	p := PlayerScore{Day: day}
	for rest := emojis; rest != ""; {
		var cluster string
		cluster, rest = nextCluster(rest)
		value := ValueOf(cluster)
		if value == -1 {
			return PlayerScore{}, &ValidationError{"unknown score type: " + cluster}
		}
		p.Choices = append(p.Choices, cluster)
		p.Score += value
	}
	return p, Validate(p)
}
