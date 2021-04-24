package baseresolver

import (
	"context"
	"io/fs"
	"sync"

	"go.etcd.io/etcd/api/v3/mvccpb"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/common/expbackoff"
)

func MakeEtcdResolveFunc(etcdClient *v3.Client, etcdPrefix string, etcdPort string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, retries *int, backoff expbackoff.ExpBackoff) (<-chan []*Event, error) {
		ch := make(chan []*Event)

		resp, err := etcdClient.KV.Get(
			ctx,
			etcdPrefix,
			v3.WithPrefix(),
			v3.WithSerializable())
		err = etcdMapError(err)
		if err != nil {
			close(ch)
			return nil, err
		}

		wch := etcdClient.Watcher.Watch(
			ctx,
			etcdPrefix,
			v3.WithPrefix(),
			v3.WithRev(resp.Header.Revision+1))

		wg.Add(1)
		go func() {
			defer func() {
				close(ch)
				wg.Done()
			}()

			if len(resp.Kvs) != 0 {
				events := make([]*Event, 0, len(resp.Kvs))
				for _, kv := range resp.Kvs {
					if ev := etcdMapEvent(etcdPort, v3.EventTypePut, kv); ev != nil {
						events = append(events, ev)
					}
				}
				ch <- events
			}

			for {
				select {
				case <-ctx.Done():
					return

				case wresp, ok := <-wch:
					if !ok {
						return
					}
					n := len(wresp.Events)
					err := etcdMapError(wresp.Err())
					if err != nil {
						n++
					}
					events := make([]*Event, 0, n)
					for _, wev := range wresp.Events {
						if ev := etcdMapEvent(etcdPort, wev.Type, wev.Kv); ev != nil {
							events = append(events, ev)
						}
					}
					if err != nil {
						ev := &Event{
							Type: ErrorEvent,
							Err:  err,
						}
						events = append(events, ev)
					}
					ch <- events
				}
			}
		}()

		return ch, nil
	}
}

func etcdMapError(err error) error {
	return err
}

func etcdMapEvent(portName string, t mvccpb.Event_EventType, kv *mvccpb.KeyValue) *Event {
	key := string(kv.Key)
	switch t {
	case v3.EventTypePut:
		return ParseServerSetData(portName, key, kv.Value)

	case v3.EventTypeDelete:
		return &Event{
			Type: DeleteEvent,
			Key:  key,
			Data: &AddrData{Err: fs.ErrNotExist},
		}

	default:
		return nil
	}
}
