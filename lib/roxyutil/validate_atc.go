package roxyutil

func ValidateATCServiceName(str string) error {
	if str == "" {
		return BadATCServiceNameError{
			ServiceName: str,
			Err:         ErrExpectNonEmpty,
		}
	}
	if !reATCService.MatchString(str) {
		return BadATCServiceNameError{
			ServiceName: str,
			Err: FailedMatchError{
				Input:   str,
				Pattern: reATCService,
			},
		}
	}
	return nil
}

func ValidateATCLocation(str string) error {
	if str == "" {
		return nil
	}
	if !reATCLocation.MatchString(str) {
		return BadATCLocationError{
			Location: str,
			Err: FailedMatchError{
				Input:   str,
				Pattern: reATCLocation,
			},
		}
	}
	return nil
}

func ValidateATCUnique(str string) error {
	if str == "" {
		return BadATCUniqueError{
			Unique: str,
			Err:    ErrExpectNonEmpty,
		}
	}
	if !reATCUnique.MatchString(str) {
		return BadATCUniqueError{
			Unique: str,
			Err: FailedMatchError{
				Input:   str,
				Pattern: reATCUnique,
			},
		}
	}
	return nil
}
