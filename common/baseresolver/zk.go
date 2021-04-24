package baseresolver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/common/expbackoff"
)

func MakeZKResolveFunc(zkconn *zk.Conn, zkPath string, zkPort string) WatchingResolveFunc {
	return func(ctx context.Context, wg *sync.WaitGroup, retries *int, backoff expbackoff.ExpBackoff) (<-chan []*Event, error) {
		var children []string
		var zch <-chan zk.Event
		var err error

		children, _, zch, err = zkconn.ChildrenW(zkPath)
		err = zkMapError(err)
		if err != nil {
			return nil, err
		}

		childEventCh := make(chan *Event)
		childDoneCh := make(chan struct{})

		// begin child thread body
		// {{{

		childThread := func(myPath string) {
			defer func() {
				childEventCh <- &Event{
					Type: ErrorEvent,
					Err:  ErrChildExited,
					Key:  myPath,
				}
				wg.Done()
			}()

			var raw []byte
			var zch <-chan zk.Event
			var err error

			retries := 0
			childSleep := func() bool {
				t := time.NewTimer(backoff.Backoff(retries))
				retries++
				select {
				case <-ctx.Done():
					t.Stop()
					return false
				case <-childDoneCh:
					t.Stop()
					return false
				case <-t.C:
					return true
				}
			}

			for {
				for {
					raw, _, zch, err = zkconn.GetW(myPath)
					err = zkMapError(err)
					if err == nil {
						break
					}
					if zkIsFatalError(err) {
						childEventCh <- &Event{
							Type: DeleteEvent,
							Key:  myPath,
							Data: &AddrData{Err: fs.ErrNotExist},
						}
						return
					}
					childEventCh <- &Event{
						Type: BadDataEvent,
						Key:  myPath,
						Data: &AddrData{Err: err},
					}
					if !childSleep() {
						return
					}
				}

				retries = 0
				childEventCh <- ParseServerSetData(zkPort, myPath, raw)

				select {
				case <-ctx.Done():
					return
				case <-childDoneCh:
					return
				case zev := <-zch:
					switch zev.Type {
					case zk.EventNodeDataChanged:
						// pass
					case zk.EventNodeDeleted:
						childEventCh <- &Event{
							Type: DeleteEvent,
							Key:  myPath,
							Data: &AddrData{Err: fs.ErrNotExist},
						}
						return
					default:
						err = zkMapError(zev.Err)
						if err != nil {
							if zkIsFatalError(err) {
								childEventCh <- &Event{
									Type: DeleteEvent,
									Key:  myPath,
									Data: &AddrData{Err: fs.ErrNotExist},
								}
								return
							}
							childEventCh <- &Event{
								Type: BadDataEvent,
								Key:  myPath,
								Data: &AddrData{Err: err},
							}
							if !childSleep() {
								return
							}
						}
					}
				}
			}
		}

		// end child thread body
		// }}}

		ch := make(chan []*Event)
		alive := make(map[string]struct{}, 16)

		for _, child := range children {
			childPath := path.Join(zkPath, child)
			alive[childPath] = struct{}{}
			wg.Add(1)
			go childThread(childPath)
		}

		// begin parent thread body
		// {{{

		wg.Add(1)
		go func() {
			defer func() {
				close(ch)
				close(childDoneCh)
				for len(alive) != 0 {
					ev := <-childEventCh
					if zkIsChildExit(ev) {
						delete(alive, ev.Key)
					}
				}
				close(childEventCh)
				wg.Done()
			}()

			for {
				select {
				case <-ctx.Done():
					return
				case ev := <-childEventCh:
					if zkIsChildExit(ev) {
						delete(alive, ev.Key)
					} else {
						ch <- []*Event{ev}
					}
				case zev := <-zch:
					switch zev.Type {
					case zk.EventNodeChildrenChanged:
						// pass
					case zk.EventNodeDeleted:
						err := fmt.Errorf("node %q was deleted: %w", zkPath, fs.ErrNotExist)
						ch <- []*Event{
							{
								Type: ErrorEvent,
								Err:  err,
							},
						}
						return
					default:
						err = zkMapError(zev.Err)
						if err != nil {
							ch <- []*Event{
								{
									Type: ErrorEvent,
									Err:  err,
								},
							}
							return
						}
					}

					children, _, zch, err = zkconn.ChildrenW(zkPath)
					err = zkMapError(err)
					if err != nil {
						ch <- []*Event{
							{
								Type: ErrorEvent,
								Err:  err,
							},
						}
						return
					}

					for _, child := range children {
						childPath := path.Join(zkPath, child)
						if _, exists := alive[childPath]; !exists {
							alive[childPath] = struct{}{}
							wg.Add(1)
							go childThread(childPath)
						}
					}
				}
			}
		}()

		// end parent thread body
		// }}}

		return ch, nil
	}
}

func zkIsChildExit(ev *Event) bool {
	return (ev != nil) && (ev.Type == ErrorEvent) && (ev.Err == ErrChildExited)
}

func zkIsFatalError(err error) bool {
	return errors.Is(err, fs.ErrClosed) || errors.Is(err, fs.ErrNotExist)
}

func zkMapError(err error) error {
	switch err {
	case zk.ErrConnectionClosed:
		return fs.ErrClosed
	case zk.ErrClosing:
		return fs.ErrClosed
	case zk.ErrNodeExists:
		return fs.ErrExist
	case zk.ErrNoNode:
		return fs.ErrNotExist
	default:
		return err
	}
}
