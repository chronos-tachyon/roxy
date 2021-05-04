package roxyresolver

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
)

func TestNewUnixResolver(t *testing.T) {
	var cancelFn context.CancelFunc

	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		ctx, cancelFn = context.WithDeadline(ctx, deadline)
		defer cancelFn()
	}

	rng := FakeRandom()

	type testRow1 struct {
		name string
		opts Options
		addr *net.UnixAddr
	}

	testData1 := []testRow1{
		{
			name: "filesystem-1",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "/path/to/socket",
				},
				Context: ctx,
				Random:  rng,
			},
			addr: &net.UnixAddr{
				Net:  "unix",
				Name: "/path/to/socket",
			},
		},
		{
			name: "filesystem-2",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "localhost",
					Endpoint:  "/path/to/socket",
				},
				Context: ctx,
				Random:  rng,
			},
			addr: &net.UnixAddr{
				Net:  "unix",
				Name: "/path/to/socket",
			},
		},
		{
			name: "abstract-1",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "@abstract-socket",
				},
				Context: ctx,
				Random:  rng,
			},
			addr: &net.UnixAddr{
				Net:  "unix",
				Name: "\x00abstract-socket",
			},
		},
		{
			name: "abstract-2",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "\x00abstract-socket",
				},
				Context: ctx,
				Random:  rng,
			},
			addr: &net.UnixAddr{
				Net:  "unix",
				Name: "\x00abstract-socket",
			},
		},
		{
			name: "abstract-3",
			opts: Options{
				Target: Target{
					Scheme:    "unix-abstract",
					Authority: "",
					Endpoint:  "abstract-socket",
				},
				Context: ctx,
				Random:  rng,
			},
			addr: &net.UnixAddr{
				Net:  "unix",
				Name: "\x00abstract-socket",
			},
		},
		{
			name: "with-balancer",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "/path/to/socket?balancer=rr",
				},
				Context: ctx,
				Random:  rng,
			},
			addr: &net.UnixAddr{
				Net:  "unix",
				Name: "/path/to/socket",
			},
		},
	}

	for _, row := range testData1 {
		t.Run(row.name, func(t *testing.T) {
			res, err := NewUnixResolver(row.opts)
			if err != nil {
				t.Errorf("NewUnixResolver failed: %v", err)
				return
			}
			records, err := res.ResolveAll()
			if err != nil {
				t.Errorf("ResolveAll failed: %v", err)
				return
			}
			if len(records) != 1 {
				t.Errorf("ResolveAll returned %d records, expected 1", len(records))
			}
			data := records[0]
			if data.Addr == nil {
				t.Error("ResolveAll.[0].Addr is nil")
				return
			}
			unixAddr, ok := data.Addr.(*net.UnixAddr)
			if !ok {
				t.Errorf("ResolveAll.[0].Addr is %T, not *net.UnixAddr", data.Addr)
				return
			}
			if !reflect.DeepEqual(*row.addr, *unixAddr) {
				t.Errorf("expected %#v, got %#v", *row.addr, *unixAddr)
				return
			}
			var events []Event
			res.Watch(func(list []Event) {
				events = list
			})
			if len(events) != 1 {
				t.Errorf("Watch yielded %d events, expected 1", len(events))
				return
			}
			event := Event{
				Type: UpdateEvent,
				Key:  data.Unique,
				Data: data,
			}
			if !reflect.DeepEqual(event, events[0]) {
				t.Errorf("expected %#v, got %#v", event, events[0])
			}
		})
	}

	type testRow2 struct {
		name  string
		opts  Options
		errfn func(error) bool
	}

	testData2 := []testRow2{
		{
			name: "err-authority-is-not-empty",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "example.com",
					Endpoint:  "/path/to/socket",
				},
				Context: ctx,
				Random:  rng,
			},
			errfn: func(err error) bool {
				var xerr BadAuthorityError
				if errors.As(err, &xerr) {
					return xerr.Err == ErrExpectEmptyOrLocalhost
				}
				return false
			},
		},
		{
			name: "err-endpoint-is-empty",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "",
				},
				Context: ctx,
				Random:  rng,
			},
			errfn: func(err error) bool {
				var xerr BadEndpointError
				if errors.As(err, &xerr) {
					return xerr.Err == ErrExpectNonEmpty
				}
				return false
			},
		},
		{
			name: "err-path-is-empty",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "?foo=bar",
				},
				Context: ctx,
				Random:  rng,
			},
			errfn: func(err error) bool {
				var xerr BadPathError
				if errors.As(err, &xerr) {
					return xerr.Err == ErrExpectNonEmpty
				}
				return false
			},
		},
		{
			name: "err-bad-query",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "/path/to/socket?foo=%",
				},
				Context: ctx,
				Random:  rng,
			},
			errfn: func(err error) bool {
				var xerr BadQueryStringError
				return errors.As(err, &xerr)
			},
		},
		{
			name: "err-bad-balancer",
			opts: Options{
				Target: Target{
					Scheme:    "unix",
					Authority: "",
					Endpoint:  "/path/to/socket?balancer=bogus",
				},
				Context: ctx,
				Random:  rng,
			},
			errfn: func(err error) bool {
				var xerr BadQueryParamError
				if errors.As(err, &xerr) {
					return xerr.Name == "balancer" && xerr.Value == "bogus"
				}
				return false
			},
		},
	}

	for _, row := range testData2 {
		t.Run(row.name, func(t *testing.T) {
			_, err := NewUnixResolver(row.opts)
			if err == nil {
				t.Errorf("NewUnixResolver unexpectedly succeeded")
			} else if !row.errfn(err) {
				t.Errorf("NewUnixResolver returned the wrong error: %v", err)
			}
		})
	}
}
