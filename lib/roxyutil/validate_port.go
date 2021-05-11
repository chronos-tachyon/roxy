package roxyutil

func ValidateNamedPort(str string) error {
	if str == "" {
		return BadPortError{
			Port: str,
			Err:  ErrExpectNonEmpty,
		}
	}
	if !reSSPort.MatchString(str) {
		return BadPortError{
			Port: str,
			Err: FailedMatchError{
				Input:   str,
				Pattern: reSSPort,
			},
		}
	}
	return nil
}
