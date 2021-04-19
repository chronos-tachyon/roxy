package balancedclient

import (
	"errors"
)

var (
	ErrClosed            = errors.New("closed")
	ErrNoHealthyBackends = errors.New("no healthy backends")
)
