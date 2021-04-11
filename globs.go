package main

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrEmptyHostGlob   = errors.New("host glob is empty")
	ErrIllegalHostGlob = errors.New("host glob must be * or [*.]some.domain.tld")

	ErrEmptyPathGlob   = errors.New("path glob is empty")
	ErrIllegalPathGlob = errors.New("path glob must be * or /a/b/*/c")
)

var (
	reNormalHostGlob = regexp.MustCompile(`^(?:\*\.)?(?:[0-9A-Za-z_-]+\.)*[0-9A-Za-z]+$`)
	reAnyHost        = regexp.MustCompile(`^(?:[0-9A-Za-z_-]+\.)*[0-9A-Za-z]+\.?$`)

	reNormalPathGlob = regexp.MustCompile(`^(?:/(?:\*\*?|[0-9A-Za-z_.~+-]+))+/?$`)
	reAnyPath        = regexp.MustCompile(`^/(.*)$`)
	reEmptyPath      = regexp.MustCompile(`^/$`)
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

func CompilePathGlob(pat string) (*regexp.Regexp, error) {
	if pat == "" {
		return nil, InvalidPathGlobError{Glob: pat, Err: ErrEmptyPathGlob}
	}

	if pat == "*" || pat == "/**" {
		return reAnyPath, nil
	}

	if pat == "/" {
		return reEmptyPath, nil
	}

	if !reNormalPathGlob.MatchString(pat) {
		return nil, InvalidPathGlobError{Glob: pat, Err: ErrIllegalPathGlob}
	}

	pieces := strings.Split(pat[1:], "/")
	var buf strings.Builder
	buf.Grow(len(pat))
	buf.WriteByte('^')
	for _, x := range pieces {
		buf.WriteByte('/')
		switch x {
		case "":
			// pass
		case "*":
			buf.WriteString(`([^/]+)`)
		case "**":
			buf.WriteString(`([^/]+(?:/[^/]+)*)`)
		default:
			buf.WriteString(regexp.QuoteMeta(x))
		}
	}
	buf.WriteByte('$')

	rx := buf.String()
	return regexp.MustCompile(rx), nil
}
