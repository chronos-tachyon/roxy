package baseresolver

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/chronos-tachyon/roxy/common/membership"
)

var (
	ErrChildExited       = errors.New("child has exited")
	ErrNoHealthyBackends = errors.New("no healthy backends")
)

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
