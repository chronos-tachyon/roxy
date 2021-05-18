package roxyutil

import (
	"path/filepath"
)

// PathAbs is a wrapper around "path/filepath".Abs.
func PathAbs(str string) (string, error) {
	abs, err := filepath.Abs(str)
	if err != nil {
		return "", PathAbsError{Path: str, Err: err}
	}
	return abs, nil
}
