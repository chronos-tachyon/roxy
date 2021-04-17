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
		buf.WriteString(err.Section)
		buf.WriteString(": ")
	}
	buf.WriteString(err.Err.Error())
	return buf.String()
}

func (err ConfigLoadError) Unwrap() error {
	return err.Err
}

var _ error = ConfigLoadError{}

// }}}

// type StorageEngineCreateError {{{

type StorageEngineCreateError struct {
	Engine string
	Err    error
}

func (err StorageEngineCreateError) Error() string {
	return fmt.Sprintf("failed to load storage engine %q: %v", err.Engine, err.Err)
}

func (err StorageEngineCreateError) Unwrap() error {
	return err.Err
}

var _ error = StorageEngineCreateError{}

// }}}

// type StorageEngineOperationError {{{

type StorageEngineOperationError struct {
	Engine string
	Op     string
	Key    string
	Err    error
}

func (err StorageEngineOperationError) Error() string {
	return fmt.Sprintf("storage engine %q: failed to %s key %q: %v", err.Engine, err.Op, err.Key, err.Err)
}

func (err StorageEngineOperationError) Unwrap() error {
	return err.Err
}

var _ error = StorageEngineOperationError{}

// }}}

// type TLSClientConfigError {{{

type TLSClientConfigError struct {
	Err error
}

func (err TLSClientConfigError) Error() string {
	return fmt.Sprintf("tls client: %v", err.Err)
}

func (err TLSClientConfigError) Unwrap() error {
	return err.Err
}

var _ error = TLSClientConfigError{}

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
