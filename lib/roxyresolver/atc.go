package roxyresolver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"strings"
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
	lbHost, lbPort, lbName, lbLocation, lbServerName, balancer, isDSC, isTLS, err := ParseATCTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	lbTarget := makeATCDialTarget(lbHost, lbPort)
	dialOpts := makeATCDialOpts(lbHost, lbServerName, isTLS)
	lbcc, err := grpc.DialContext(opts.Context, lbTarget, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to Dial %q: %w", lbTarget, err)
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     opts.Context,
		Random:      opts.Random,
		Balancer:    balancer,
		ResolveFunc: MakeATCResolveFunc(lbcc, lbName, lbLocation, isDSC),
	})
}

func ParseATCTarget(target Target) (lbHost string, lbPort string, lbName string, lbLocation string, lbServerName string, balancer BalancerType, isDSC bool, isTLS bool, err error) {
	if target.Authority == "" {
		err = BadAuthorityError{Authority: target.Authority, Err: ErrExpectNonEmpty}
		return
	}

	var unescaped string
	unescaped, err = url.PathUnescape(target.Authority)
	if err != nil {
		err = BadAuthorityError{Authority: target.Authority, Err: err}
		return
	}

	lbHost, lbPort, err = net.SplitHostPort(unescaped)
	if err != nil {
		h, p, err2 := net.SplitHostPort(unescaped + ":" + atcPort)
		if err2 == nil {
			lbHost, lbPort, err = h, p, nil
		}
		if err != nil {
			err = BadAuthorityError{
				Authority: target.Authority,
				Err:       BadHostPortError{HostPort: unescaped, Err: err},
			}
			return
		}
	}
	if !reLBHost.MatchString(lbHost) {
		err = BadAuthorityError{
			Authority: target.Authority,
			Err:       BadHostError{Host: lbHost, Err: ErrFailedToMatch},
		}
		return
	}
	if !reLBPort.MatchString(lbPort) {
		err = BadAuthorityError{
			Authority: target.Authority,
			Err:       BadPortError{Port: lbPort, Err: ErrFailedToMatch},
		}
		return
	}

	ep := target.Endpoint
	if ep == "" {
		err = BadEndpointError{Endpoint: target.Endpoint, Err: ErrExpectNonEmpty}
		return
	}

	var (
		qs    string
		hasQS bool
	)
	if i := strings.IndexByte(ep, '?'); i >= 0 {
		ep, qs, hasQS = ep[:i], ep[i+1:], true
	}

	if ep == "" {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err:      BadPathError{Path: ep, Err: ErrExpectNonEmpty},
		}
		return
	}

	lbName, err = url.PathUnescape(ep)
	if err != nil {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err:      BadPathError{Path: ep, Err: err},
		}
		return
	}
	if !reLBName.MatchString(lbName) {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadPathError{
				Path: ep,
				Err:  BadNameError{Name: lbName, Err: ErrFailedToMatch},
			},
		}
		return
	}

	var query url.Values
	if hasQS {
		query, err = url.ParseQuery(qs)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err:      BadQueryStringError{QueryString: qs, Err: err},
			}
			return
		}
	}

	if str := query.Get("balancer"); str != "" {
		err = balancer.Parse(str)
		if err != nil {
			err = BadEndpointError{
				Endpoint: target.Endpoint,
				Err: BadQueryStringError{
					QueryString: qs,
					Err: BadQueryParamError{
						Name:  "balancer",
						Value: str,
						Err:   err,
					},
				},
			}
			return
		}
	}

	str := query.Get("disableServiceConfig")
	isDSC, err = ParseBool(str, false)
	if err != nil {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadQueryStringError{
				QueryString: qs,
				Err: BadQueryParamError{
					Name:  "disableServiceConfig",
					Value: str,
					Err:   err,
				},
			},
		}
		return
	}

	lbLocation = query.Get("location")
	if lbLocation == "" {
		lbLocation = query.Get("loc")
	}

	str = query.Get("tls")
	isTLS, err = ParseBool(str, true)
	if err != nil {
		err = BadEndpointError{
			Endpoint: target.Endpoint,
			Err: BadQueryStringError{
				QueryString: qs,
				Err: BadQueryParamError{
					Name:  "tls",
					Value: str,
					Err:   err,
				},
			},
		}
		return
	}

	lbServerName = query.Get("lbServerName")

	return
}

func MakeATCResolveFunc(lbcc *grpc.ClientConn, lbName string, lbLocation string, dsc bool) WatchingResolveFunc {
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
						resAddr := Address{
							Addr:       tcpAddr.String(),
							ServerName: e.ServerName,
						}
						data := Resolved{
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
	ctx context.Context
	rng *rand.Rand
}

func (b atcBuilder) Scheme() string {
	return atcScheme
}

func (b atcBuilder) Build(target Target, cc grpcresolver.ClientConn, opts grpcresolver.BuildOptions) (grpcresolver.Resolver, error) {
	lbHost, lbPort, lbName, lbLocation, lbServerName, _, _, isTLS, err := ParseATCTarget(target)
	if err != nil {
		return nil, err
	}

	lbTarget := makeATCDialTarget(lbHost, lbPort)
	dialOpts := makeATCDialOpts(lbHost, lbServerName, isTLS)
	lbcc, err := grpc.DialContext(b.ctx, lbTarget, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to Dial %q: %w", lbTarget, err)
	}

	return NewWatchingResolver(WatchingResolverOptions{
		Context:     b.ctx,
		Random:      b.rng,
		ResolveFunc: MakeATCResolveFunc(lbcc, lbName, lbLocation, opts.DisableServiceConfig),
		ClientConn:  cc,
	})
}

var _ grpcresolver.Builder = atcBuilder{}

// }}}

func makeATCDialTarget(lbHost, lbPort string) string {
	return "dns:///" + net.JoinHostPort(lbHost, lbPort)
}

func makeATCDialOpts(lbHost, lbServerName string, isTLS bool) []grpc.DialOption {
	out := make([]grpc.DialOption, 1)
	if isTLS {
		serverName := lbHost
		if lbServerName != "" {
			serverName = lbServerName
		}
		tlsConfig := &tls.Config{ServerName: serverName}
		out[0] = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	} else {
		out[0] = grpc.WithInsecure()
	}
	return out
}
