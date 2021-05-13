package announcer

import (
	"context"
	"errors"
	"io"
	"io/fs"

	multierror "github.com/hashicorp/go-multierror"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

type stateType uint8

const (
	stateInit stateType = iota
	stateRunning
	stateDead
	stateClosed
)

type Impl interface {
	Announce(ctx context.Context, r *membership.Roxy) error
	Withdraw(ctx context.Context) error
	Close() error
}

type Announcer struct {
	impls []Impl
	alive []bool
	state stateType
}

func New() *Announcer {
	return &Announcer{}
}

func (a *Announcer) Add(impl Impl) {
	if impl == nil {
		panic(errors.New("Impl is nil"))
	}

	checkAnnounce(a.state)

	a.impls = append(a.impls, impl)
}

func (a *Announcer) Announce(ctx context.Context, r *membership.Roxy) error {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if r == nil {
		panic(errors.New("*membership.Roxy is nil"))
	}

	checkAnnounce(a.state)

	a.state = stateRunning
	a.alive = make([]bool, len(a.impls))
	var errs multierror.Error
	for index, impl := range a.impls {
		err := impl.Announce(ctx, r)
		if err == nil {
			a.alive[index] = true
		}
		if isRealError(err) {
			errs.Errors = append(errs.Errors, err)
		}
	}
	return errs.ErrorOrNil()
}

func (a *Announcer) Withdraw(ctx context.Context) error {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}

	checkWithdraw(a.state)

	var errs multierror.Error
	for index, impl := range a.impls {
		if a.alive[index] {
			err := impl.Withdraw(ctx)
			if isRealError(err) {
				errs.Errors = append(errs.Errors, err)
			}
		}
	}
	a.alive = nil
	a.state = stateInit
	return errs.ErrorOrNil()
}

func (a *Announcer) Close() error {
	var needWithdraw bool

	switch a.state {
	case stateInit:
		// pass
	case stateRunning:
		fallthrough
	case stateDead:
		needWithdraw = true
	default:
		return fs.ErrClosed
	}

	var errs multierror.Error
	if needWithdraw {
		for index, impl := range a.impls {
			if a.alive[index] {
				err := impl.Withdraw(context.Background())
				if isRealError(err) {
					errs.Errors = append(errs.Errors, err)
				}
			}
		}
	}
	for _, impl := range a.impls {
		if err := impl.Close(); isRealError(err) {
			errs.Errors = append(errs.Errors, err)
		}
	}
	a.alive = nil
	a.state = stateClosed
	return errs.ErrorOrNil()
}

func isRealError(err error) bool {
	switch {
	case err == nil:
		return false
	case err == io.EOF:
		return false
	case errors.Is(err, fs.ErrClosed):
		return false
	default:
		return true
	}
}
