package balancedclient

import (
	"crypto/tls"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

func NewATCResolver(opts Options) (baseresolver.Resolver, error) {
	lbHost, lbPort, lbName, query, err := baseresolver.ParseATCTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	var balancer baseresolver.BalancerType
	if str := query.Get("balancer"); str != "" {
		if err = balancer.Parse(str); err != nil {
			return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", str, err)
		}
	}

	isTLS, err := baseresolver.ParseBool(query.Get("tls"), true)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tls=%q query string: %w", query.Get("tls"), err)
	}

	isDSC, err := baseresolver.ParseBool(query.Get("disableServiceConfig"), false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse disableServiceConfig=%q query string: %w", query.Get("disableServiceConfig"), err)
	}

	dialOpts := make([]grpc.DialOption, 1)
	if isTLS {
		serverName := lbHost
		if str := query.Get("serverName"); str != "" {
			serverName = str
		}
		tlsConfig := &tls.Config{ServerName: serverName}
		dialOpts[0] = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	} else {
		dialOpts[0] = grpc.WithInsecure()
	}

	lbTarget := "dns:///" + net.JoinHostPort(lbHost, lbPort)
	lbcc, err := grpc.DialContext(opts.Context, lbTarget, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to Dial %q: %w", lbTarget, err)
	}

	return baseresolver.NewWatchingResolver(baseresolver.WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: baseresolver.MakeATCResolveFunc(lbcc, lbName, isDSC),
	})
}
