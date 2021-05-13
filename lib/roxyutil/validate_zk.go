package roxyutil

import (
	"strings"
)

// ValidateZKPath validates that the given string is a valid ZooKeeper node
// path.
func ValidateZKPath(str string) error {
	if str == "" {
		return BadPathError{Path: str, Err: ErrExpectNonEmpty}
	}
	if str[0] != '/' {
		return BadPathError{Path: str, Err: ErrExpectLeadingSlash}
	}
	if str != "/" && strings.HasSuffix(str, "/") {
		return BadPathError{Path: str, Err: ErrExpectNoEndSlash}
	}
	if strings.Contains(str, "//") {
		return BadPathError{Path: str, Err: ErrExpectNoDoubleSlash}
	}
	if strings.Contains(str+"/", "/./") {
		return BadPathError{Path: str, Err: ErrExpectNoDot}
	}
	if strings.Contains(str+"/", "/../") {
		return BadPathError{Path: str, Err: ErrExpectNoDotDot}
	}
	return nil
}
