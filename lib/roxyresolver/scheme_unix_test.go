package roxyresolver

import (
	"context"
	"errors"
	"net"
	"net/url"
	"reflect"
	"testing"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

func TestUnixTarget_RoundTrip(t *testing.T) {
	type testRow struct {
		Input   string
		RoxyIn  Target
		Unix    UnixTarget
		RoxyOut Target
		Output  string
	}

	testData := []testRow{
		{
			Input: "unix:/path/to/socket",
			RoxyIn: Target{
				Scheme:     constants.SchemeUnix,
				Endpoint:   "/path/to/socket",
				ServerName: "localhost",
				HasSlash:   false,
			},
			Unix: UnixTarget{
				Addr:       &net.UnixAddr{Net: constants.NetUnix, Name: "/path/to/socket"},
				IsAbstract: false,
				ServerName: "localhost",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeUnix,
				Endpoint:   "/path/to/socket",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "localhost",
				HasSlash:   true,
			},
			Output: "unix:////path/to/socket?balancer=random",
		},
		{
			Input: "unix:/path/to/socket?serverName=example.com&balancer=rr",
			RoxyIn: Target{
				Scheme:     constants.SchemeUnix,
				Endpoint:   "/path/to/socket",
				ServerName: "example.com",
				Query:      url.Values{"balancer": []string{"rr"}, "serverName": []string{"example.com"}},
				HasSlash:   false,
			},
			Unix: UnixTarget{
				Addr:       &net.UnixAddr{Net: constants.NetUnix, Name: "/path/to/socket"},
				IsAbstract: false,
				ServerName: "example.com",
				Balancer:   RoundRobinBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeUnix,
				Endpoint:   "/path/to/socket",
				Query:      url.Values{"balancer": []string{"roundRobin"}, "serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Output: "unix:////path/to/socket?balancer=roundRobin&serverName=example.com",
		},
		{
			Input: "unix://localhost/path/to/socket?serverName=example.com",
			RoxyIn: Target{
				Scheme:     constants.SchemeUnix,
				Endpoint:   "/path/to/socket",
				ServerName: "example.com",
				Query:      url.Values{"serverName": []string{"example.com"}},
				HasSlash:   true,
			},
			Unix: UnixTarget{
				Addr:       &net.UnixAddr{Net: constants.NetUnix, Name: "/path/to/socket"},
				IsAbstract: false,
				ServerName: "example.com",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeUnix,
				Endpoint:   "/path/to/socket",
				Query:      url.Values{"balancer": []string{"random"}, "serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Output: "unix:////path/to/socket?balancer=random&serverName=example.com",
		},
		{
			Input: "unix:@abstract-name",
			RoxyIn: Target{
				Scheme:     constants.SchemeUnixAbstract,
				Endpoint:   "abstract-name",
				ServerName: "localhost",
				HasSlash:   false,
			},
			Unix: UnixTarget{
				Addr:       &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-name"},
				IsAbstract: true,
				ServerName: "localhost",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeUnixAbstract,
				Endpoint:   "abstract-name",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "localhost",
				HasSlash:   true,
			},
			Output: "unix-abstract:///abstract-name?balancer=random",
		},
		{
			Input: "unix-abstract:abstract-name?serverName=localhost",
			RoxyIn: Target{
				Scheme:     constants.SchemeUnixAbstract,
				Endpoint:   "abstract-name",
				Query:      url.Values{"serverName": []string{"localhost"}},
				ServerName: "localhost",
				HasSlash:   false,
			},
			Unix: UnixTarget{
				Addr:       &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-name"},
				IsAbstract: true,
				ServerName: "localhost",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeUnixAbstract,
				Endpoint:   "abstract-name",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "localhost",
				HasSlash:   true,
			},
			Output: "unix-abstract:///abstract-name?balancer=random",
		},
		{
			Input: "unix-abstract://localhost/abstract-name",
			RoxyIn: Target{
				Scheme:     constants.SchemeUnixAbstract,
				Endpoint:   "abstract-name",
				ServerName: "localhost",
				HasSlash:   true,
			},
			Unix: UnixTarget{
				Addr:       &net.UnixAddr{Net: constants.NetUnix, Name: "\x00abstract-name"},
				IsAbstract: true,
				ServerName: "localhost",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeUnixAbstract,
				Endpoint:   "abstract-name",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "localhost",
				HasSlash:   true,
			},
			Output: "unix-abstract:///abstract-name?balancer=random",
		},
	}

	for _, row := range testData {
		t.Run(row.Input, func(t *testing.T) {
			var roxyIn Target
			if err := roxyIn.Parse(row.Input); err != nil {
				t.Errorf("Target.Parse failed: %v", err)
				return
			}
			if !reflect.DeepEqual(roxyIn, row.RoxyIn) {
				t.Errorf("Target.Parse returned %#v, expected %#v", roxyIn, row.RoxyIn)
			}

			var target UnixTarget
			if err := target.FromTarget(roxyIn); err != nil {
				t.Errorf("UnixTarget.FromTarget failed: %v", err)
				return
			}
			if !reflect.DeepEqual(target, row.Unix) {
				t.Errorf("UnixTarget.FromTarget returned %#v, expected %#v", target, row.Unix)
			}

			roxyOut := target.AsTarget()
			if !reflect.DeepEqual(roxyOut, row.RoxyOut) {
				t.Errorf("UnixTarget.AsTarget returned %#v, expected %#v", roxyOut, row.RoxyOut)
			}

			output := roxyOut.String()
			if output != row.Output {
				t.Errorf("Target.String returned %q, expected %q", output, row.Output)
			}
		})
	}
}

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
				Key:  data.UniqueID,
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
