package roxyresolver

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"
	grpcresolver "google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/lib/expbackoff"
	"github.com/chronos-tachyon/roxy/roxypb"
)

func NewATCBuilder(ctx context.Context, rng *rand.Rand, lbcc *grpc.ClientConn) grpcresolver.Builder {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}
	if lbcc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}
	return atcBuilder{ctx, rng, lbcc}
}

func NewATCResolver(opts Options) (Resolver, error) {
	if opts.Context == nil {
		panic(errors.New("context.Context is nil"))
	}

	lbcc := GetATCClient(opts.Context)
	if lbcc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}

	lbName, lbLocation, balancer, isDSC, serverName, err := ParseATCTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeATCResolveFunc(lbcc, lbName, lbLocation, isDSC, serverName),
	})
}

func ParseATCTarget(rt RoxyTarget) (lbName string, lbLocation string, balancer BalancerType, isDSC bool, serverName string, err error) {
	if rt.Authority != "" {
		err = BadAuthorityError{Authority: rt.Authority, Err: ErrExpectEmpty}
		return
	}

	lbName = rt.Endpoint
	if lbName == "" {
		err = BadEndpointError{Endpoint: rt.Endpoint, Err: ErrExpectNonEmpty}
		return
	}
	err = ValidateATCServiceName(lbName)
	if err != nil {
		err = BadEndpointError{Endpoint: rt.Endpoint, Err: err}
		return
	}

	if str := rt.Query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = BadQueryParamError{Name: "balancer", Value: str, Err: err}
			return
		}
	}

	if str := rt.Query.Get("disableServiceConfig"); str != "" {
		isDSC, err = ParseBool(str)
		if err != nil {
			err = BadQueryParamError{Name: "disableServiceConfig", Value: str, Err: err}
			return
		}
	}

	lbLocation = rt.Query.Get("location")
	if lbLocation == "" {
		lbLocation = rt.Query.Get("loc")
	}
	err = ValidateATCLocation(lbLocation)
	if err != nil {
		err = BadQueryParamError{Name: "location", Value: lbLocation, Err: err}
		return
	}

	serverName = rt.Query.Get("serverName")

	return
}

func ValidateATCServiceName(lbName string) error {
	if lbName == "" {
		return BadServiceNameError{ServiceName: lbName, Err: ErrExpectNonEmpty}
	}
	if !reLBName.MatchString(lbName) {
		return BadServiceNameError{ServiceName: lbName, Err: ErrFailedToMatch}
	}
	return nil
}

func ValidateATCLocation(lbLocation string) error {
	if lbLocation == "" {
		return nil
	}
	if !reLBLoc.MatchString(lbLocation) {
		return BadLocationError{Location: lbLocation, Err: ErrFailedToMatch}
	}
	return nil
}

func MakeATCResolveFunc(lbcc *grpc.ClientConn, lbName string, lbLocation string, dsc bool, serverName string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, _ expbackoff.ExpBackoff) (<-chan []Event, error) {
		lbclient := roxypb.NewAirTrafficControlClient(lbcc)

		bc, err := lbclient.Balance(ctx)
		if err != nil {
			lbcc.Close()
			return nil, err
		}

		err = bc.Send(&roxypb.BalanceRequest{
			Name:     lbName,
			Location: lbLocation,
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

		ch := make(chan []Event)
		activeCh := make(chan struct{})

		// begin send thread
		// {{{

		wg.Add(1)
		go func() {
			defer func() {
				_ = bc.CloseSend()
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
			byUnique := make(map[string]Resolved, 16)

			defer func() {
				if len(byUnique) != 0 {
					events := make([]Event, 0, len(byUnique))
					for _, data := range byUnique {
						events = append(events, Event{
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
					ch <- []Event{
						{
							Type: ErrorEvent,
							Err:  err,
						},
					}
					return
				}

				var events []Event
				for _, e := range resp.Events {
					switch e.EventType {
					case roxypb.Event_INSERT_IP:
						tcpAddr := &net.TCPAddr{
							IP:   net.IP(e.Ip),
							Port: int(e.Port),
							Zone: e.Zone,
						}
						myServerName := e.ServerName
						if myServerName == "" {
							myServerName = serverName
						}
						if myServerName == "" {
							myServerName = tcpAddr.IP.String()
						}
						resAddr := Address{
							Addr:       tcpAddr.String(),
							ServerName: myServerName,
						}
						data := Resolved{
							Unique:     e.Unique,
							Location:   e.Location,
							ServerName: myServerName,
							ShardID:    e.ShardId,
							Weight:     e.Weight,
							HasShardID: resp.IsSharded,
							HasWeight:  true,
							Addr:       tcpAddr,
							Address:    resAddr,
						}
						byUnique[data.Unique] = data
						events = append(events, Event{
							Type: UpdateEvent,
							Key:  data.Unique,
							Data: data,
						})

					case roxypb.Event_DELETE_IP:
						delete(byUnique, e.Unique)
						events = append(events, Event{
							Type: DeleteEvent,
							Key:  e.Unique,
						})

					case roxypb.Event_UPDATE_WEIGHT:
						old := byUnique[e.Unique]
						data := Resolved{
							Unique:     old.Unique,
							Location:   old.Location,
							ServerName: old.ServerName,
							ShardID:    old.ShardID,
							Weight:     e.Weight,
							HasShardID: old.HasShardID,
							HasWeight:  true,
							Addr:       old.Addr,
							Address:    old.Address,
							Dynamic:    old.Dynamic,
						}
						byUnique[data.Unique] = data
						events = append(events, Event{
							Type: StatusChangeEvent,
							Key:  data.Unique,
							Data: data,
						})

					case roxypb.Event_NEW_SERVICE_CONFIG:
						events = append(events, Event{
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
	ctx  context.Context
	rng  *rand.Rand
	lbcc *grpc.ClientConn
}

func (b atcBuilder) Scheme() string {
	return atcScheme
}

func (b atcBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	rt, err := RoxyTargetFromTarget(target)
	if err != nil {
		return nil, err
	}

	lbName, lbLocation, _, _, serverName, err := ParseATCTarget(rt)
	if err != nil {
		return nil, err
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     b.ctx,
		Random:      b.rng,
		ResolveFunc: MakeATCResolveFunc(b.lbcc, lbName, lbLocation, opts.DisableServiceConfig, serverName),
		ClientConn:  cc,
	})
}

var _ grpcresolver.Builder = atcBuilder{}

// }}}
