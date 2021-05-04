package main

import (
	"errors"
	"regexp"
)

var (
	ErrEmptyHostGlob   = errors.New("host glob is empty")
	ErrIllegalHostGlob = errors.New("host glob must be * or [*.]some.domain.tld")
)

var (
	reNormalHostGlob = regexp.MustCompile(`^(?:\*\.)?(?:[0-9A-Za-z_-]+\.)*[0-9A-Za-z]+$`)
	reAnyHost        = regexp.MustCompile(`^(?:[0-9A-Za-z_-]+\.)*[0-9A-Za-z]+\.?$`)
)

func CompileHostGlob(pat string) (*regexp.Regexp, error) {
	if pat == "" {
		return nil, InvalidHostGlobError{Glob: pat, Err: ErrEmptyHostGlob}
	}

	if pat == "*" {
		return reAnyHost, nil
	}

	if !reNormalHostGlob.MatchString(pat) {
		return nil, InvalidHostGlobError{Glob: pat, Err: ErrIllegalHostGlob}
	}

	if pat[0] == '*' {
		rx := `^(?i)(?:[0-9A-Za-z_-]+\.)*` + regexp.QuoteMeta(pat[2:]) + `\.?$`
		return regexp.MustCompile(rx), nil
	}

	rx := `^(?i)` + regexp.QuoteMeta(pat) + `\.?$`
	return regexp.MustCompile(rx), nil
}
