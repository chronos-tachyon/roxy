package baseresolver

import (
	"context"
	"io"
	"reflect"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/common/expbackoff"
	"github.com/chronos-tachyon/roxy/roxypb"
)

func MakeATCResolveFunc(lbcc *grpc.ClientConn, lbName string, dsc bool) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, retries *int, backoff expbackoff.ExpBackoff) (<-chan []*Event, error) {
		lbclient := roxypb.NewAirTrafficControlClient(lbcc)

		bc, err := lbclient.Balance(ctx)
		if err != nil {
			lbcc.Close()
			return nil, err
		}

		err = bc.Send(&roxypb.BalanceRequest{Name: lbName})
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
			var (
				oldList []*AddrData          = nil
				oldMap  map[string]*AddrData = nil
			)

			defer func() {
				if len(oldList) != 0 {
					events := make([]*Event, len(oldList))
					for index, data := range oldList {
						dataKey := data.Key()
						events[index] = &Event{
							Type: DeleteEvent,
							Key: dataKey,
							Data: data,
						}
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
					ch <- []*Event{{Type: ErrorEvent, Err: err}}
					return
				}

				var (
					newList = make([]*AddrData, 0, len(resp.Endpoints))
					newMap  = make(map[string]*AddrData, len(resp.Endpoints))
				)
				for _, ep := range resp.Endpoints {
					ev := ParseATCEndpointData(ep)
					newList = append(newList, ev.Data)
					newMap[ev.Key] = ev.Data
				}

				hasServiceConfig := !dsc && (len(resp.ServiceConfig) != 0)

				n := len(oldList) + len(newList)
				if hasServiceConfig {
					n++
				}

				events := make([]*Event, 0, n)
				if hasServiceConfig {
					events = append(events, &Event{
						Type:              NewServiceConfigEvent,
						ServiceConfigJSON: resp.ServiceConfig,
					})
				}
				for _, oldData := range oldList {
					dataKey := oldData.Key()
					if _, found := newMap[dataKey]; found {
						continue
					}
					events = append(events, &Event{
						Type: DeleteEvent,
						Key:  dataKey,
						Data: oldData,
					})
				}
				for _, newData := range newList {
					dataKey := newData.Key()
					ev := &Event{
						Type: UpdateEvent,
						Key:  dataKey,
						Data: newData,
					}
					if oldData, found := oldMap[dataKey]; found {
						if reflect.DeepEqual(oldData, newData) {
							continue
						}
						oldAddrKey := oldData.Addr.String()
						newAddrKey := newData.Addr.String()
						if oldAddrKey == newAddrKey {
							ev.Type = StatusChangeEvent
						}
					}
					events = append(events, ev)
				}
				if len(events) != 0 {
					ch <- events
				}

				oldList = newList
				oldMap = newMap
			}
		}()

		// end recv thread
		// }}}

		return ch, nil
	}
}
