package balancedclient

import (
	"errors"
)

var ErrNoHealthyBackends = errors.New("no healthy backends")
