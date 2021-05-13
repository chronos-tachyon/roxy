package roxyutil

// ValidateATCServiceName validates that the given string is a valid Service
// Name for the Air Traffic Control service.
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

// ValidateATCServiceName validates that the given string is a valid Location
// for the Air Traffic Control service.
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

// ValidateATCServiceName validates that the given string is a valid Unique ID
// for the Air Traffic Control service.
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
