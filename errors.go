package main

import (
	"fmt"
	"strings"
)

// type ConfigLoadError {{{

type ConfigLoadError struct {
	Path    string
	Section string
	Err     error
}

func (err ConfigLoadError) Error() string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "failed to load config file %q: ", err.Path)
	if err.Section != "" {
		fmt.Fprintf(&buf, "%s: ", err.Section)
	}
	buf.WriteString(err.Err.Error())
	return buf.String()
}

func (err ConfigLoadError) Unwrap() error {
	return err.Err
}

var _ error = ConfigLoadError{}

// }}}

// type InvalidHostGlobError {{{

type InvalidHostGlobError struct {
	Glob string
	Err  error
}

func (err InvalidHostGlobError) Error() string {
	return fmt.Sprintf("invalid host glob %q: %v", err.Glob, err.Err)
}

func (err InvalidHostGlobError) Unwrap() error {
	return err.Err
}

var _ error = InvalidHostGlobError{}

// }}}

// type InvalidPathGlobError {{{

type InvalidPathGlobError struct {
	Glob string
	Err  error
}

func (err InvalidPathGlobError) Error() string {
	return fmt.Sprintf("invalid path glob %q: %v", err.Glob, err.Err)
}

func (err InvalidPathGlobError) Unwrap() error {
	return err.Err
}

var _ error = InvalidPathGlobError{}

// }}}
