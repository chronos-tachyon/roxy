package roxyresolver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/roxypb"
)

func NewATCBuilder(ctx context.Context, rng *rand.Rand) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("ctx is nil"))
	}
	return atcBuilder{ctx, rng}
}

func NewATCResolver(opts Options) (Resolver, error) {
	lbHost, lbPort, lbName, query, err := ParseATCTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	var balancer BalancerType
	if str := query.Get("balancer"); str != "" {
		if err = balancer.Parse(str); err != nil {
			return nil, fmt.Errorf("failed to parse balancer=%q query string: %w", str, err)
		}
	}

	isDSC, err := ParseBool(query.Get("disableServiceConfig"), false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse disableServiceConfig=%q query string: %w", query.Get("disableServiceConfig"), err)
	}

	lbLoc := query.Get("location")
	if lbLoc == "" {
		lbLoc = query.Get("loc")
	}

	isTLS, err := ParseBool(query.Get("tls"), true)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tls=%q query string: %w", query.Get("tls"), err)
	}

	dialOpts := make([]grpc.DialOption, 1)
	if isTLS {
		serverName := lbHost
		if str := query.Get("lbServerName"); str != "" {
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

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeATCResolveFunc(lbcc, lbName, lbLoc, isDSC),
	})
}

func MakeATCResolveFunc(lbcc *grpc.ClientConn, lbName string, lbLoc string, dsc bool) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []*Event, error) {
		lbclient := roxypb.NewAirTrafficControlClient(lbcc)

		bc, err := lbclient.Balance(ctx)
		if err != nil {
			lbcc.Close()
			return nil, err
		}

		err = bc.Send(&roxypb.BalanceRequest{
			Name:     lbName,
			Location: lbLoc,
		})
		if err != nil {
			var errs multierror.Error
			errs.Errors = make([]error, 1, 2)
			errs.Errors[0] = err

			err = bc.CloseSend()
			if err != nil {
				errs.Errors = append(errs.Errors, err)
			}

			lbcc.Close()
			return nil, errs.ErrorOrNil()
		}

		ch := make(chan []*Event)
		activeCh := make(chan struct{})

		// begin send thread
		// {{{

		wg.Add(1)
		go func() {
			defer func() {
				bc.CloseSend()
				<-activeCh
				lbcc.Close()
				wg.Done()
			}()

			select {
			case <-ctx.Done():
				return
			case <-activeCh:
				return
			}
		}()

		// }}}

		// begin recv thread
		// {{{

		wg.Add(1)
		go func() {
			byUnique := make(map[string]*Resolved, 16)

			defer func() {
				if len(byUnique) != 0 {
					events := make([]*Event, 0, len(byUnique))
					for _, data := range byUnique {
						events = append(events, &Event{
							Type: DeleteEvent,
							Key:  data.Unique,
						})
					}
					ch <- events
				}
				close(activeCh)
				wg.Done()
			}()

			for {
				resp, err := bc.Recv()
				if err == io.EOF {
					return
				}
				if err != nil {
					ch <- []*Event{
						{
							Type: ErrorEvent,
							Err:  err,
						},
					}
					return
				}

				var events []*Event
				for _, e := range resp.Events {
					switch e.EventType {
					case roxypb.Event_INSERT_IP:
						tcpAddr := &net.TCPAddr{
							IP:   net.IP(e.Ip),
							Port: int(e.Port),
							Zone: e.Zone,
						}
						resAddr := Address{
							Addr:       tcpAddr.String(),
							ServerName: e.ServerName,
						}
						data := &Resolved{
							Unique:     e.Unique,
							Location:   e.Location,
							ServerName: e.ServerName,
							ShardID:    e.ShardId,
							Weight:     e.Weight,
							HasShardID: resp.IsSharded,
							HasWeight:  true,
							Addr:       tcpAddr,
							Address:    resAddr,
						}
						byUnique[data.Unique] = data
						events = append(events, &Event{
							Type: UpdateEvent,
							Key:  data.Unique,
							Data: data,
						})

					case roxypb.Event_DELETE_IP:
						delete(byUnique, e.Unique)
						events = append(events, &Event{
							Type: DeleteEvent,
							Key:  e.Unique,
						})

					case roxypb.Event_UPDATE_WEIGHT:
						old := byUnique[e.Unique]
						data := &Resolved{
							Unique:     old.Unique,
							Location:   old.Location,
							ServerName: old.ServerName,
							ShardID:    old.ShardID,
							Weight:     e.Weight,
							HasShardID: old.HasShardID,
							HasWeight:  true,
							Addr:       old.Addr,
							Address:    old.Address,
						}
						data.UpdateFrom(old)
						byUnique[data.Unique] = data
						events = append(events, &Event{
							Type: StatusChangeEvent,
							Key:  data.Unique,
							Data: data,
						})

					case roxypb.Event_NEW_SERVICE_CONFIG:
						events = append(events, &Event{
							Type:              NewServiceConfigEvent,
							ServiceConfigJSON: e.ServiceConfigJson,
						})
					}
				}
				if len(events) != 0 {
					ch <- events
				}
			}
		}()

		// end recv thread
		// }}}

		return ch, nil
	}
}

// type atcBuilder {{{

type atcBuilder struct {
	ctx context.Context
	rng *rand.Rand
}

func (b atcBuilder) Scheme() string {
	return "atc"
}

func (b atcBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	lbHost, lbPort, lbName, query, err := ParseATCTarget(target)
	if err != nil {
		return nil, err
	}

	lbLoc := query.Get("location")
	if lbLoc == "" {
		lbLoc = query.Get("loc")
	}

	isTLS, err := ParseBool(query.Get("tls"), true)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tls=%q query string: %w", query.Get("tls"), err)
	}

	dialOpts := make([]grpc.DialOption, 1)
	if isTLS {
		serverName := lbHost
		if str := query.Get("lbServerName"); str != "" {
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

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     b.ctx,
		Random:      b.rng,
		ResolveFunc: MakeATCResolveFunc(lbcc, lbName, lbLoc, opts.DisableServiceConfig),
		ClientConn:  cc,
	})
}

var _ grpcresolver.Builder = atcBuilder{}

// }}}
