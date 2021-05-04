package announcer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/roxypb"
)

type LoadFunc func() float32

var DefaultLoadFunc LoadFunc = func() float32 { return 0.0 }

func (a *Announcer) AddATC(cc *grpc.ClientConn, name, unique, location, portName string, loadFn LoadFunc) error {
	impl, err := NewATC(cc, name, unique, location, portName, loadFn)
	if err != nil {
		return err
	}
	a.Add(impl)
	return nil
}

func NewATC(cc *grpc.ClientConn, name, unique, location, portName string, loadFn LoadFunc) (Impl, error) {
	if cc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}
	if name == "" {
		return nil, fmt.Errorf("invalid LB target name %q", name)
	}
	if unique == "" {
		return nil, fmt.Errorf("invalid unique string %q", unique)
	}
	if loadFn == nil {
		loadFn = DefaultLoadFunc
	}
	return &atcImpl{
		cc:       cc,
		atc:      roxypb.NewAirTrafficControlClient(cc),
		name:     name,
		unique:   unique,
		location: location,
		portName: portName,
		loadFn:   loadFn,
	}, nil
}

type atcImpl struct {
	wg       sync.WaitGroup
	cc       *grpc.ClientConn
	atc      roxypb.AirTrafficControlClient
	name     string
	unique   string
	location string
	portName string
	loadFn   LoadFunc
	rc       roxypb.AirTrafficControl_ReportClient
	doneCh   chan struct{}
}

func (impl *atcImpl) Announce(ctx context.Context, ss *membership.ServerSet) error {
	tcpAddr := ss.TCPAddrForPort(impl.portName)
	if tcpAddr == nil {
		return fmt.Errorf("unknown port name %q", impl.portName)
	}

	var (
		serverName string
		shardID    int32
		hasShardID bool
	)

	rc, err := impl.atc.Report(ctx)
	if err != nil {
		return err
	}

	err = rc.Send(&roxypb.ReportRequest{
		Load: impl.loadFn(),
		FirstReport: &roxypb.ReportRequest_FirstReport{
			Name:       impl.name,
			Unique:     impl.unique,
			Location:   impl.location,
			ServerName: serverName,
			ShardId:    shardID,
			Ip:         []byte(tcpAddr.IP),
			Zone:       tcpAddr.Zone,
			Port:       uint32(tcpAddr.Port),
			HasShardId: hasShardID,
		},
	})
	if err != nil {
		rc.CloseAndRecv()
		return err
	}

	impl.rc = rc
	impl.doneCh = make(chan struct{})

	impl.wg.Add(1)
	go func() {
		t := time.NewTicker(30 * time.Second)
		looping := true
		for looping {
			select {
			case <-ctx.Done():
				looping = false
			case <-impl.doneCh:
				looping = false
			case <-t.C:
				err := rc.Send(&roxypb.ReportRequest{
					Load: impl.loadFn(),
				})
				if err != nil {
					looping = false
				}
			}
		}
		t.Stop()
		impl.wg.Done()
	}()

	return nil
}

func (impl *atcImpl) Withdraw(ctx context.Context) error {
	if impl.rc == nil {
		return nil
	}
	close(impl.doneCh)
	_, err := impl.rc.CloseAndRecv()
	impl.rc = nil
	impl.doneCh = nil
	return err
}

func (impl *atcImpl) Close() error {
	impl.wg.Wait()
	return nil
}

var _ Impl = (*atcImpl)(nil)
