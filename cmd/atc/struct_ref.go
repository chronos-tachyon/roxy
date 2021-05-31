package main

import (
	"context"
	"io/fs"
	"net"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/rs/zerolog/log"
	v3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type Ref struct {
	cfg    *GlobalConfigFile
	zkConn *zk.Conn
	etcd   *v3.Client

	wg          sync.WaitGroup
	mu          sync.Mutex
	cv1         *sync.Cond
	cv2         *sync.Cond
	shardsByKey map[Key]*ShardData
	next        map[uint64]*Impl
	curr        *Impl
	refCount    uint32
	waiters     uint32
	exclusive   bool
	closed      bool
	closeCh     chan struct{}
}

func (ref *Ref) Init(file *GlobalConfigFile, zkConn *zk.Conn, etcd *v3.Client) {
	roxyutil.AssertNotNil(&file)

	ref.cfg = file
	ref.zkConn = zkConn
	ref.etcd = etcd
	ref.cv1 = sync.NewCond(&ref.mu)
	ref.cv2 = sync.NewCond(&ref.mu)
	ref.shardsByKey = make(map[Key]*ShardData, 16)
	ref.closeCh = make(chan struct{})

	ref.wg.Add(1)
	go ref.periodicThread()
}

func (ref *Ref) GlobalConfig() *GlobalConfigFile {
	return ref.cfg
}

func (ref *Ref) ZKConn() *zk.Conn {
	return ref.zkConn
}

func (ref *Ref) EtcdV3Client() *v3.Client {
	return ref.etcd
}

func (ref *Ref) AcquireSharedImpl() *Impl {
	ref.mu.Lock()
	for ref.exclusive || ref.waiters != 0 {
		ref.cv1.Wait()
	}
	ref.refCount++
	impl := ref.curr
	ref.mu.Unlock()
	return impl
}

func (ref *Ref) ReleaseSharedImpl() {
	ref.mu.Lock()
	ref.refCount--
	if ref.refCount == 0 {
		ref.cv2.Signal()
	}
	ref.mu.Unlock()
}

func (ref *Ref) lockedAcquireExclusiveImpl() {
	ref.waiters++
	for ref.exclusive || ref.refCount != 0 {
		ref.cv2.Wait()
	}
	ref.waiters--
	ref.exclusive = true
}

func (ref *Ref) lockedReleaseExclusiveImpl() {
	ref.exclusive = false
	if ref.waiters == 0 {
		ref.cv1.Broadcast()
	} else {
		ref.cv2.Signal()
	}
}

func (ref *Ref) Load(ctx context.Context, configID uint64, rev int64) error {
	roxyutil.AssertNotNil(&ctx)

	next, err := ref.loadImpl(ctx, rev)
	if err != nil {
		return err
	}

	ref.mu.Lock()
	ref.lockedAcquireExclusiveImpl()
	if ref.next == nil {
		ref.next = make(map[uint64]*Impl, 1)
	}
	ref.next[configID] = next
	ref.lockedReleaseExclusiveImpl()
	ref.mu.Unlock()

	return nil
}

func (ref *Ref) Flip(ctx context.Context, configID uint64) error {
	roxyutil.AssertNotNil(&ctx)

	ref.mu.Lock()
	ref.lockedAcquireExclusiveImpl()

	defer func() {
		ref.lockedReleaseExclusiveImpl()
		ref.mu.Unlock()
	}()

	curr := ref.curr
	next := ref.next[configID]

	if next == nil {
		return status.Errorf(codes.NotFound, "config_id %d is not loaded", configID)
	}

	if next == curr {
		return status.Errorf(codes.InvalidArgument, "config_id %d is already live", configID)
	}

	ref.curr = next

	if len(ref.shardsByKey) == 0 {
		return nil
	}

	type migrationData struct {
		PeerData      *PeerData
		ShardDataList []*ShardData
	}

	migrationDataByAddr := make(map[*net.TCPAddr]*migrationData, len(next.PeerList))
	for key, shardData := range ref.shardsByKey {
		svc := next.ServiceMap.Get(key.ServiceName)
		shardLimit := ShardID(svc.EffectiveNumShards())

		shardData.Mutex.Lock()

		shardData.ServiceData = svc

		switch {
		case key.ShardID >= shardLimit:
			for _, clientData := range shardData.ClientsByUnique {
				clientData.LockedSendGoAway(nil) // force immediate codes.NotFound
			}
			for _, serverData := range shardData.ServersByUnique {
				serverData.LockedSendGoAway(nil) // force immediate codes.NotFound
			}

		case next.SelfData.Contains(key):
			// pass

		default:
			peerData := next.PeerByKey(key)
			md := migrationDataByAddr[peerData.Addr]
			if md == nil {
				md = &migrationData{PeerData: peerData}
				migrationDataByAddr[peerData.Addr] = md
			}
			md.ShardDataList = append(md.ShardDataList, shardData)
		}

		shardData.Mutex.Unlock()
	}

	var wg sync.WaitGroup
	for _, md := range migrationDataByAddr {
		wg.Add(1)
		go ref.migrationThread(ctx, &wg, configID, md.PeerData, md.ShardDataList)
	}

	ref.mu.Unlock()
	wg.Wait()
	ref.mu.Lock()

	return nil
}

func (ref *Ref) Commit(ctx context.Context, configID uint64) error {
	roxyutil.AssertNotNil(&ctx)

	ref.mu.Lock()
	ref.lockedAcquireExclusiveImpl()

	defer func() {
		ref.lockedReleaseExclusiveImpl()
		ref.mu.Unlock()
	}()

	curr := ref.curr
	next := ref.next[configID]

	if next == nil {
		return status.Errorf(codes.NotFound, "config_id %d is not loaded", configID)
	}

	if next != curr {
		return status.Errorf(codes.InvalidArgument, "config_id %d is not live", configID)
	}

	delete(ref.next, configID)
	return nil
}

func (ref *Ref) Shard(key Key) *ShardData {
	ref.mu.Lock()
	shardData := ref.shardsByKey[key]
	ref.mu.Unlock()
	return shardData
}

func (ref *Ref) GetOrInsertShard(key Key, svc *ServiceData) *ShardData {
	roxyutil.AssertNotNil(&svc)

	ref.mu.Lock()
	shardData := ref.lockedGetOrInsertShard(key, svc)
	ref.mu.Unlock()
	return shardData
}

func (ref *Ref) lockedGetOrInsertShard(key Key, svc *ServiceData) *ShardData {
	roxyutil.AssertNotNil(&svc)

	shardData := ref.shardsByKey[key]
	if shardData == nil {
		shardData = &ShardData{
			ServiceName:     key.ServiceName,
			ShardID:         key.ShardID,
			HasShardID:      svc.IsSharded,
			ServiceData:     svc,
			ClientsByUnique: make(map[string]*ClientData, svc.ExpectedNumClientsPerShard),
			ServersByUnique: make(map[string]*ServerData, svc.ExpectedNumServersPerShard),
		}
		shardData.Cond = sync.NewCond(&shardData.Mutex)
		ref.shardsByKey[key] = shardData
	} else {
		shardData.Mutex.Lock()
		shardData.ServiceData = svc
		shardData.Mutex.Unlock()
	}
	return shardData
}

func (ref *Ref) PeerConn(
	ctx context.Context,
	addr *net.TCPAddr,
) (
	cc *grpc.ClientConn,
	atc roxy_v0.AirTrafficControlClient,
	err error,
) {
	roxyutil.AssertNotNil(&ctx)
	roxyutil.AssertNotNil(&addr)

	var tlsDialOpt grpc.DialOption
	tlsDialOpt, err = ref.cfg.PeerTLS.MakeDialOption(addr.IP.String())
	if err != nil {
		return nil, nil, err
	}

	dialOpts := make([]grpc.DialOption, 4)
	dialOpts[0] = roxyresolver.WithStandardResolvers(ctx)
	dialOpts[1] = tlsDialOpt
	dialOpts[2] = grpc.WithBlock()
	dialOpts[3] = grpc.FailOnNonTempDialError(true)

	cc, err = grpc.DialContext(ctx, addr.String(), dialOpts...)
	if err != nil {
		return nil, nil, err
	}

	atc = roxy_v0.NewAirTrafficControlClient(cc)
	return cc, atc, nil
}

func (ref *Ref) Close() error {
	ref.mu.Lock()
	ref.lockedAcquireExclusiveImpl()

	defer func() {
		ref.lockedReleaseExclusiveImpl()
		ref.mu.Unlock()
	}()

	if ref.closed {
		return fs.ErrClosed
	}
	ref.closed = true
	close(ref.closeCh)
	ref.wg.Wait()

	return nil
}

func (ref *Ref) loadImpl(ctx context.Context, rev int64) (*Impl, error) {
	impl := &Impl{GlobalConfig: ref.cfg}

	err := impl.doLoad(ctx, ref.zkConn, ref.etcd, rev)
	if err != nil {
		return nil, err
	}

	return impl, nil
}

func (ref *Ref) migrationThread(
	ctx context.Context,
	wg *sync.WaitGroup,
	configID uint64,
	peerData *PeerData,
	shardDataList []*ShardData,
) {
	defer wg.Done()

	goAway := peerData.GoAway()

	cc, atc, err := ref.PeerConn(ctx, peerData.Addr)
	if err != nil {
		log.Logger.Error().
			Stringer("addr", peerData.Addr).
			Err(err).
			Msg("Ref.PeerConn failed")
	}

	defer func() {
		if err := cc.Close(); err != nil {
			log.Logger.Error().
				Stringer("addr", peerData.Addr).
				Err(err).
				Msg("grpc.ClientConn.Close failed")
		}
	}()

	for _, shardData := range shardDataList {
		if cc != nil {
			shardData.Mutex.Lock()

			req := &roxy_v0.TransferRequest{
				ConfigId:    configID,
				ServiceName: string(shardData.ServiceName),
				ShardId:     uint32(shardData.ShardID),
				HasShardId:  shardData.HasShardID,
				Clients:     make([]*roxy_v0.ClientData, 0, len(shardData.ClientsByUnique)),
				Servers:     make([]*roxy_v0.ServerData, 0, len(shardData.ServersByUnique)),
			}

			for _, clientData := range shardData.ClientsByUnique {
				samples := clientData.CostHistory.Snapshot()

				client := &roxy_v0.ClientData{
					Unique:                clientData.Unique,
					Location:              string(clientData.Location),
					DeclaredCostPerSecond: clientData.DeclaredCPS,
					History:               make([]*roxy_v0.Sample, len(samples)),
				}

				for index, sample := range samples {
					client.History[index] = sample.ToProto()
				}

				req.Clients = append(req.Clients, client)
			}

			for _, serverData := range shardData.ServersByUnique {
				samples := serverData.CostHistory.Snapshot()

				server := &roxy_v0.ServerData{
					Unique:                serverData.Unique,
					Location:              string(serverData.Location),
					ServerName:            serverData.ServerName,
					Ip:                    []byte(serverData.Addr.IP),
					Zone:                  serverData.Addr.Zone,
					Port:                  uint32(serverData.Addr.Port),
					DeclaredCostPerSecond: serverData.DeclaredCPS,
					History:               make([]*roxy_v0.Sample, len(samples)),
				}

				for index, sample := range samples {
					server.History[index] = sample.ToProto()
				}

				req.Servers = append(req.Servers, server)
			}

			shardData.Mutex.Unlock()

			_, err := atc.Transfer(ctx, req)
			if err != nil {
				log.Logger.Error().
					Stringer("addr", peerData.Addr).
					Stringer("key", shardData.Key()).
					Err(err).
					Msg("AirTrafficControl.Transfer RPC failed")
			}
		}

		shardData.Mutex.Lock()
		for _, clientData := range shardData.ClientsByUnique {
			clientData.LockedSendGoAway(goAway)
		}
		for _, serverData := range shardData.ServersByUnique {
			serverData.LockedSendGoAway(goAway)
		}
		shardData.Mutex.Unlock()
	}
}

func (ref *Ref) periodicThread() {
	defer ref.wg.Done()
	for {
		t := time.NewTimer(PeriodicInterval)
		select {
		case <-ref.closeCh:
			t.Stop()
			return

		case <-t.C:
			ref.periodic()
		}
	}
}

func (ref *Ref) periodic() {
	ref.mu.Lock()
	ref.lockedPeriodic()
	ref.mu.Unlock()
}

func (ref *Ref) lockedPeriodic() {
	for key, shardData := range ref.shardsByKey {
		shardData.Mutex.Lock()
		shardData.LockedPeriodic()
		if len(shardData.ClientsByUnique) == 0 && len(shardData.ServersByUnique) == 0 {
			delete(ref.shardsByKey, key)
		}
		shardData.Mutex.Unlock()
	}
}
