package roxyresolver

import (
	"fmt"
	"io/fs"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

// type ResolveError {{{

// ResolveError indicates a problem with a specific address.
type ResolveError struct {
	UniqueID string
	Err      error
}

// Error fulfills the error interface.
func (err ResolveError) Error() string {
	return fmt.Sprintf("%q: %v", err.UniqueID, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err ResolveError) Unwrap() error {
	return err.Err
}

var _ error = ResolveError{}

// }}}

// type NodeDeletedError {{{

// NodeDeletedError indicates that a server is in a bad state.
type NodeDeletedError struct {
	Path string
}

// Error fulfills the error interface.
func (err NodeDeletedError) Error() string {
	return fmt.Sprintf("node %q was deleted", err.Path)
}

// Is returns true for fs.ErrNotExist.
func (err NodeDeletedError) Is(other error) bool {
	return other == fs.ErrNotExist
}

var _ error = NodeDeletedError{}

// }}}

// type StatusError {{{

// StatusError indicates that a server is in a bad state.
type StatusError struct {
	Status membership.ServerSetStatus
}

// Error fulfills the error interface.
func (err StatusError) Error() string {
	return fmt.Sprintf("status is %v", err.Status)
}

// Is returns true for fs.ErrNotExist.
func (err StatusError) Is(other error) bool {
	return other == fs.ErrNotExist
}

var _ error = StatusError{}

// }}}

// type childExitError {{{

type childExitError struct {
	Path string
}

// Error fulfills the error interface.
func (err childExitError) Error() string {
	return fmt.Sprintf("child thread for path %q has exited", err.Path)
}

var _ error = childExitError{}

// }}}
