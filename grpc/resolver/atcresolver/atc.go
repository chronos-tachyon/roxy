package atcresolver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
)

func NewBuilder(ctx context.Context, rng *rand.Rand) resolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	return myBuilder{ctx, rng}
}

type myBuilder struct {
	ctx context.Context
	rng *rand.Rand
}

func (b myBuilder) Scheme() string {
	return "atc"
}

func (b myBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	lbHost, lbPort, lbName, query, err := baseresolver.ParseATCTarget(target)
	if err != nil {
		return nil, err
	}

	isTLS, err := baseresolver.ParseBool(query.Get("tls"), true)
	if err != nil {
		return nil, err
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
	lbcc, err := grpc.DialContext(b.ctx, lbTarget, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to Dial %q: %w", lbTarget, err)
	}

	return baseresolver.NewWatchingResolver(baseresolver.WatchingResolverOptions{
		Context:     b.ctx,
		Random:      b.rng,
		ResolveFunc: baseresolver.MakeATCResolveFunc(lbcc, lbName, opts.DisableServiceConfig),
		ClientConn:  cc,
	})
}

var _ resolver.Builder = myBuilder{}
