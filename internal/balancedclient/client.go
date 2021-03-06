package balancedclient

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
)

func New(
	res roxyresolver.Resolver,
	interceptor atcclient.InterceptorFactory,
	dialer *net.Dialer,
	tlsConfig *tls.Config,
) (*BalancedClient, error) {
	if res == nil {
		panic(errors.New("roxyresolver.Resolver is nil"))
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

	checkRedirect := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	var roundTripper http.RoundTripper
	roundTripper = &http.Transport{
		DialContext:           dialFunc,
		DialTLSContext:        dialFunc,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	roundTripper = interceptor.RoundTripper(roundTripper)

	bc := &BalancedClient{
		res: res,
		client: &http.Client{
			Transport:     roundTripper,
			CheckRedirect: checkRedirect,
		},
		isTLS: (tlsConfig != nil),
	}
	return bc, nil
}

// type BalancedClient {{{

type BalancedClient struct {
	res    roxyresolver.Resolver
	client *http.Client
	isTLS  bool
}

func (bc *BalancedClient) Resolver() roxyresolver.Resolver {
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
