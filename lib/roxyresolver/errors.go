package roxyresolver

import (
	"fmt"
	"io/fs"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

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

// type childExitError {{{

type childExitError struct {
	Path string
}

func (err childExitError) Error() string {
	return fmt.Sprintf("child thread for path %q has exited", err.Path)
}

var _ error = childExitError{}

// }}}
