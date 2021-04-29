package balancedclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-zookeeper/zk"
	v3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

type Options struct {
	Target    resolver.Target
	Context   context.Context
	Random    *rand.Rand
	Etcd      *v3.Client
	ZK        *zk.Conn
	Dialer    *net.Dialer
	TLSConfig *tls.Config
}

func New(opts Options) (*BalancedClient, error) {
	res, err := NewResolver(opts)
	if err != nil {
		return nil, err
	}
	return NewWithResolver(res, opts.Dialer, opts.TLSConfig)
}

func NewResolver(opts Options) (baseresolver.Resolver, error) {
	scheme := strings.ToLower(opts.Target.Scheme)
	switch scheme {
	case "unix":
		fallthrough
	case "unix-abstract":
		return NewUnixResolver(opts)
	case "":
		fallthrough
	case "dns":
		return NewDNSResolver(opts)
	case "srv":
		return NewSRVResolver(opts)
	case "etcd":
		return NewEtcdResolver(opts)
	case "zk":
		return NewZKResolver(opts)
	case "atc":
		return NewATCResolver(opts)
	default:
		return nil, fmt.Errorf("Target.Scheme %q is not supported", opts.Target.Scheme)
	}
}

func NewWithResolver(res baseresolver.Resolver, dialer *net.Dialer, tlsConfig *tls.Config) (*BalancedClient, error) {
	if res == nil {
		panic(fmt.Errorf("Resolver is nil"))
	}

	if dialer == nil {
		dialer = &net.Dialer{}
	}

	dialFunc := func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		data, err := res.Resolve()
		if err != nil {
			return nil, err
		}
		var socket net.Conn
		socket, err = dialer.DialContext(ctx, data.Addr.Network(), data.Addr.String())
		if err != nil {
			return nil, err
		}
		if tlsConfig != nil {
			socket = tls.Client(socket, tlsConfig)
		}
		return socket, err
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
		isTLS: (tlsConfig != nil),
	}
	return bc, nil
}

// type BalancedClient {{{

type BalancedClient struct {
	res    baseresolver.Resolver
	client *http.Client
	isTLS  bool
}

func (bc *BalancedClient) Resolver() baseresolver.Resolver {
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
	bc.res.Close()
	bc.client.CloseIdleConnections()
	return nil
}

// }}}
