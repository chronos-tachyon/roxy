package roxyutil

import (
	"strings"
)

func ValidateEtcdPath(str string) error {
	if !strings.HasSuffix(str, "/") {
		return BadPathError{Path: str, Err: ErrExpectTrailingSlash}
	}
	if strings.Contains(str, "//") {
		return BadPathError{Path: str, Err: ErrExpectNoDoubleSlash}
	}
	if strings.Contains("/"+str, "/./") {
		return BadPathError{Path: str, Err: ErrExpectNoDot}
	}
	if strings.Contains("/"+str, "/../") {
		return BadPathError{Path: str, Err: ErrExpectNoDotDot}
	}
	return nil
}
