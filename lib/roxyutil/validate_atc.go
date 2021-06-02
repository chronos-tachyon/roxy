package roxyutil

// ValidateATCServiceName validates that the given string is a valid Service
// Name for the Air Traffic Control service.
func ValidateATCServiceName(str string) error {
	if str == "" {
		return ATCServiceNameError{
			ServiceName: str,
			Err:         ErrExpectNonEmpty,
		}
	}
	if !reATCService.MatchString(str) {
		return ATCServiceNameError{
			ServiceName: str,
			Err: RegexpMatchError{
				Input:   str,
				Pattern: reATCService,
			},
		}
	}
	return nil
}

// ValidateATCLocation validates that the given string is a valid Location for
// the Air Traffic Control service.
func ValidateATCLocation(str string) error {
	if str == "" {
		return nil
	}
	if !reATCLocation.MatchString(str) {
		return ATCLocationError{
			Location: str,
			Err: RegexpMatchError{
				Input:   str,
				Pattern: reATCLocation,
			},
		}
	}
	return nil
}

// ValidateATCUniqueID validates that the given string is a valid UniqueID for
// the Air Traffic Control service.
func ValidateATCUniqueID(str string) error {
	if str == "" {
		return ATCUniqueIDError{
			UniqueID: str,
			Err:      ErrExpectNonEmpty,
		}
	}
	if !reATCUnique.MatchString(str) {
		return ATCUniqueIDError{
			UniqueID: str,
			Err: RegexpMatchError{
				Input:   str,
				Pattern: reATCUnique,
			},
		}
	}
	return nil
}
