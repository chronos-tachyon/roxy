package roxyresolver

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

var ErrNoHealthyBackends = errors.New("no healthy backends")

// type BadStatusError {{{

type BadStatusError struct {
	Status membership.ServerSetStatus
}

func (err BadStatusError) Error() string {
	return fmt.Sprintf("status is %v", err.Status)
}

func (err BadStatusError) Is(other error) bool {
	switch other {
	case err:
		return true
	case fs.ErrNotExist:
		return true
	default:
		return false
	}
}

var _ error = BadStatusError{}

// }}}

// type ChildExitError {{{

type ChildExitError struct {
	Path string
}

func (err ChildExitError) Error() string {
	return fmt.Sprintf("child thread for path %q has exited", err.Path)
}

var _ error = ChildExitError{}

// }}}
