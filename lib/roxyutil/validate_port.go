package roxyutil

// ValidateNamedPort validates that the given string is a valid named port for
// a membership.Roxy or membership.ServerSet address advertisement.
func ValidateNamedPort(str string) error {
	if str == "" {
		return PortError{
			Port: str,
			Err:  ErrExpectNonEmpty,
		}
	}
	if !reSSPort.MatchString(str) {
		return PortError{
			Port: str,
			Err: RegexpMatchError{
				Input:   str,
				Pattern: reSSPort,
			},
		}
	}
	return nil
}
