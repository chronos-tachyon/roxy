package roxyresolver

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
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
		name   string
		target string
		addr   *net.UnixAddr
	}

	testData1 := []testRow1{
		{
			name:   "filesystem-1",
			target: "unix:/path/to/socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "/path/to/socket"},
		},
		{
			name:   "filesystem-2",
			target: "unix:///path/to/socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "/path/to/socket"},
		},
		{
			name:   "filesystem-3",
			target: "unix://localhost/path/to/socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "/path/to/socket"},
		},
		{
			name:   "abstract-1",
			target: "unix:@abstract-socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-socket"},
		},
		{
			name:   "abstract-2",
			target: "unix:\x00abstract-socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-socket"},
		},
		{
			name:   "abstract-3",
			target: "unix:///@abstract-socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-socket"},
		},
		{
			name:   "abstract-4",
			target: "unix:///\x00abstract-socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-socket"},
		},
		{
			name:   "abstract-5",
			target: "unix-abstract:abstract-socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-socket"},
		},
		{
			name:   "abstract-6",
			target: "unix-abstract:///abstract-socket",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-socket"},
		},
		{
			name:   "with-balancer",
			target: "unix:/path/to/socket?balancer=rr",
			addr:   &net.UnixAddr{Net: constants.NetUnix, Name: "/path/to/socket"},
		},
	}

	for _, row := range testData1 {
		t.Run(row.name, func(t *testing.T) {
			var rt Target
			err := rt.Parse(row.target)
			if err != nil {
				t.Errorf("Target.Parse failed: %v", err)
				return
			}

			res, err := New(Options{
				Context: ctx,
				Random:  rng,
				Target:  rt,
			})
			if err != nil {
				t.Errorf("New failed: %v", err)
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
		name   string
		target string
		errfn  func(error) bool
	}

	testData2 := []testRow2{
		{
			name:   "err-authority-is-not-empty",
			target: "unix://example.com/path/to/socket",
			errfn: func(err error) bool {
				var xerr roxyutil.AuthorityError
				if errors.As(err, &xerr) {
					return xerr.Err == roxyutil.ErrExpectEmptyOrLocalhost
				}
				return false
			},
		},
		{
			name:   "err-endpoint-is-empty-1",
			target: "unix:",
			errfn: func(err error) bool {
				var xerr roxyutil.EndpointError
				if errors.As(err, &xerr) {
					return xerr.Err == roxyutil.ErrExpectNonEmpty
				}
				return false
			},
		},
		{
			name:   "err-endpoint-is-empty-2",
			target: "unix://localhost",
			errfn: func(err error) bool {
				var xerr roxyutil.EndpointError
				if errors.As(err, &xerr) {
					return xerr.Err == roxyutil.ErrExpectNonEmpty
				}
				return false
			},
		},
		{
			name:   "err-path-is-empty",
			target: "unix:?foo=bar",
			errfn: func(err error) bool {
				var xerr roxyutil.EndpointError
				if errors.As(err, &xerr) {
					return xerr.Err == roxyutil.ErrExpectNonEmpty
				}
				return false
			},
		},
		{
			name:   "err-bad-query",
			target: "unix:/path/to/socket?foo=%",
			errfn: func(err error) bool {
				var xerr roxyutil.QueryStringError
				return errors.As(err, &xerr)
			},
		},
		{
			name:   "err-bad-balancer",
			target: "unix:/path/to/socket?balancer=bogus",
			errfn: func(err error) bool {
				var xerr roxyutil.QueryParamError
				if errors.As(err, &xerr) {
					return xerr.Name == "balancer" && xerr.Value == "bogus"
				}
				return false
			},
		},
	}

	for _, row := range testData2 {
		t.Run(row.name, func(t *testing.T) {
			var rt Target
			err := rt.Parse(row.target)
			if err != nil {
				if row.errfn(err) {
					return
				}
				t.Errorf("Target.Parse returned the wrong error: %v", err)
				return
			}

			_, err = New(Options{
				Context: ctx,
				Random:  rng,
				Target:  rt,
			})
			if err != nil {
				if row.errfn(err) {
					return
				}
				t.Errorf("New returned the wrong error: %v", err)
				return
			}
			t.Errorf("New unexpectedly succeeded")
		})
	}
}
