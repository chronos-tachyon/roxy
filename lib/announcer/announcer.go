package announcer

import (
	"context"
	"errors"
	"io/fs"

	multierror "github.com/hashicorp/go-multierror"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

const (
	stateInit = iota
	stateRunning
	stateClosed
)

type Impl interface {
	Announce(ctx context.Context, ss *membership.ServerSet) error
	Withdraw(ctx context.Context) error
	Close() error
}

type Announcer struct {
	impls []Impl
	state uint8
}

func New() *Announcer {
	return &Announcer{}
}

func (a *Announcer) Add(impl Impl) {
	if impl == nil {
		panic(errors.New("Impl is nil"))
	}
	switch a.state {
	case stateInit:
		a.impls = append(a.impls, impl)
	case stateRunning:
		panic(errors.New("Announcer.Announce has already been called"))
	default:
		panic(errors.New("Announcer.Close has already been called"))
	}
}

func (a *Announcer) Announce(ctx context.Context, ss *membership.ServerSet) error {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if ss == nil {
		panic(errors.New("*membership.ServerSet is nil"))
	}
	switch a.state {
	case stateInit:
		a.state = stateRunning
		var errs multierror.Error
		for _, impl := range a.impls {
			if err := impl.Announce(ctx, ss); isRealError(err) {
				errs.Errors = append(errs.Errors, err)
			}
		}
		return errs.ErrorOrNil()
	case stateRunning:
		panic(errors.New("Announcer.Announce has already been called"))
	default:
		panic(errors.New("Announcer.Close has already been called"))
	}
}

func (a *Announcer) Withdraw(ctx context.Context) error {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	switch a.state {
	case stateInit:
		panic(errors.New("Announcer.Announce has not yet been called"))
	case stateRunning:
		a.state = stateInit
		var errs multierror.Error
		for _, impl := range a.impls {
			if err := impl.Withdraw(ctx); isRealError(err) {
				errs.Errors = append(errs.Errors, err)
			}
		}
		return errs.ErrorOrNil()
	default:
		panic(errors.New("Announcer.Close has already been called"))
	}
}

func (a *Announcer) Close() error {
	var needWithdraw bool
	switch a.state {
	case stateInit:
		// pass
	case stateRunning:
		needWithdraw = true
	default:
		return fs.ErrClosed
	}
	a.state = stateClosed
	var errs multierror.Error
	if needWithdraw {
		for _, impl := range a.impls {
			if err := impl.Withdraw(context.Background()); isRealError(err) {
				errs.Errors = append(errs.Errors, err)
			}
		}
	}
	for _, impl := range a.impls {
		if err := impl.Close(); isRealError(err) {
			errs.Errors = append(errs.Errors, err)
		}
	}
	return errs.ErrorOrNil()
}

func isRealError(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, fs.ErrClosed):
		return false
	default:
		return true
	}
}
