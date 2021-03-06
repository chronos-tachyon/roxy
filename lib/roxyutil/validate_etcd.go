package roxyutil

import (
	"strings"
)

// ValidateEtcdPath validates that the given string is a valid etcd.io V3 key
// prefix, which must end with a "/".
func ValidateEtcdPath(str string) error {
	if !strings.HasSuffix(str, "/") {
		return PathError{Path: str, Err: ErrExpectTrailingSlash}
	}
	if strings.Contains(str, "//") {
		return PathError{Path: str, Err: ErrExpectNoDoubleSlash}
	}
	if strings.Contains("/"+str, "/./") {
		return PathError{Path: str, Err: ErrExpectNoDot}
	}
	if strings.Contains("/"+str, "/../") {
		return PathError{Path: str, Err: ErrExpectNoDotDot}
	}
	return nil
}
