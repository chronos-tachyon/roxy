package roxyresolver

import (
	"fmt"
	"io/fs"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

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

func (err childExitError) Error() string {
	return fmt.Sprintf("child thread for path %q has exited", err.Path)
}

var _ error = childExitError{}

// }}}
