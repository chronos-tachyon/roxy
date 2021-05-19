package announcer

import (
	"context"
	"errors"
	"sync"

	multierror "github.com/hashicorp/go-multierror"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/membership"
)

// Interface is the interface provided by announcers for specific protocols.
type Interface interface {
	Announce(ctx context.Context, r *membership.Roxy) error
	Withdraw(ctx context.Context) error
	Close() error
}

// Announcer is a multiplexer implementation of Interface that distributes
// method calls to multiple announcers.
type Announcer struct {
	mu    sync.Mutex
	impls []Interface
	alive []bool
	state State
}

// State returns the current state of the Announcer.
func (a *Announcer) State() State {
	a.mu.Lock()
	state := a.state
	a.mu.Unlock()
	return state
}

// Add adds a child announcer.  Both Announcer and the child announcer must be
// in the ready state, or Add will panic.
func (a *Announcer) Add(impl Interface) {
	if impl == nil {
		panic(errors.New("Interface is nil"))
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	checkAnnounce(a.state)

	a.impls = append(a.impls, impl)
}

// Announce broadcasts to all child announcers that the service is available at
// the given address.
//
// Announcer must be in the ready state, or Announce will panic.  A successful
// call to this method changes Announcer's state from the ready state to the
// running state.
func (a *Announcer) Announce(ctx context.Context, r *membership.Roxy) error {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if r == nil {
		panic(errors.New("*membership.Roxy is nil"))
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	checkAnnounce(a.state)

	var errs multierror.Error

	alive := make([]bool, len(a.impls))
	rollback := false
	for index, impl := range a.impls {
		if err := impl.Announce(ctx, r); err != nil {
			errs.Errors = append(errs.Errors, err)
			rollback = true
			break
		}
		alive[index] = true
	}

	if rollback {
		for index, impl := range a.impls {
			if alive[index] {
				if err := impl.Withdraw(ctx); err != nil {
					errs.Errors = append(errs.Errors, err)
				}
			}
		}
		return misc.ErrorOrNil(errs)
	}

	a.alive = alive
	a.state = StateRunning
	return nil
}

// Withdraw broadcasts to all child announcers that the service is no longer
// available.
//
// Announcer must be in the running state, or Withdraw will panic.  Any call to
// this method changes Announcer's state from the running state to the ready
// state.
func (a *Announcer) Withdraw(ctx context.Context) error {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	checkWithdraw(a.state)

	var errs multierror.Error

	for index, impl := range a.impls {
		if a.alive[index] {
			if err := impl.Withdraw(ctx); err != nil {
				errs.Errors = append(errs.Errors, err)
			}
		}
	}

	a.alive = nil
	a.state = StateReady
	return misc.ErrorOrNil(errs)
}

// Close broadcasts to all child announcers that no further announcements are
// coming and that they should clean up any resources.
//
// If Announcer is already in the closed state, nothing is done and
// fs.ErrClosed is returned.  If Announcer is in neither the ready state nor
// the closed state, Close will panic.  Any call to this method changes
// Announcer's state to the closed state.
func (a *Announcer) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := checkClose(a.state); err != nil {
		return err
	}

	var errs multierror.Error

	for _, impl := range a.impls {
		if err := impl.Close(); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}
	a.alive = nil
	a.state = StateClosed
	return misc.ErrorOrNil(errs)
}
