package balancedclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

func New(restype enums.ResolverType, opts Options) (*BalancedClient, error) {
	res, err := NewResolver(restype, opts)
	if err != nil {
		return nil, err
	}
	return NewWithResolver(res, opts.Dialer, opts.TLSConfig)
}

func NewResolver(restype enums.ResolverType, opts Options) (Resolver, error) {
	switch restype {
	case enums.UnixResolver:
		return NewUnixResolver(opts)
	case enums.DefaultResolver:
		fallthrough
	case enums.DNSResolver:
		return NewDNSResolver(opts)
	case enums.SRVResolver:
		return NewSRVResolver(opts)
	case enums.EtcdResolver:
		return NewEtcdResolver(opts)
	case enums.ZookeeperResolver:
		return NewZKResolver(opts)
	default:
		return nil, fmt.Errorf("%#v not supported", restype)
	}
}

func NewWithResolver(res Resolver, dialer *net.Dialer, tlsconfig *tls.Config) (*BalancedClient, error) {
	if res == nil {
		panic(fmt.Errorf("Resolver is nil"))
	}

	if dialer == nil {
		dialer = &net.Dialer{}
	}

	dialFunc := func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		addr, err := res.Resolve()
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, addr.Network(), addr.String())
	}

	if tlsconfig != nil {
		dialFunc = func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			addr, err := res.Resolve()
			if err != nil {
				return nil, err
			}
			socket, err := dialer.DialContext(ctx, addr.Network(), addr.String())
			if err != nil {
				return nil, err
			}
			return tls.Client(socket, tlsconfig), nil
		}
	}

	bc := &BalancedClient{
		res: res,
		client: &http.Client{
			Transport: &http.Transport{
				DialContext:           dialFunc,
				DialTLSContext:        dialFunc,
				ForceAttemptHTTP2:     true,
				DisableCompression:    true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		isTLS: (tlsconfig != nil),
	}
	return bc, nil
}

// type BalancedClient {{{

type BalancedClient struct {
	res    Resolver
	client *http.Client
	isTLS  bool
}

func (bc *BalancedClient) Resolver() Resolver {
	return bc.res
}

func (bc *BalancedClient) HTTPClient() *http.Client {
	return bc.client
}

func (bc *BalancedClient) IsTLS() bool {
	return bc.isTLS
}

func (bc *BalancedClient) Do(req *http.Request) (*http.Response, error) {
	return bc.client.Do(req)
}

func (bc *BalancedClient) Close() error {
	err := bc.res.Close()
	bc.client.CloseIdleConnections()
	return err
}

// }}}
