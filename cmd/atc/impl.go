package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/go-zookeeper/zk"
	multierror "github.com/hashicorp/go-multierror"
	v3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

const MaxNumATCs = uint(1 << 16)

type Ref struct {
	cfg    *GlobalConfigFile
	zkConn *zk.Conn
	etcd   *v3.Client

	mu          sync.Mutex
	cv          *sync.Cond
	peerConnMap map[*net.TCPAddr]*PeerConnData
	live        *Impl
	next        *Impl
	sendDone    bool
	recvDone    bool
}

type PeerConnData struct {
	cc     *grpc.ClientConn
	refcnt uint32
}

type Impl struct {
	ref *Ref

	cfgPeers    PeersFile
	cfgServices ServicesFile
	cfgCost     CostFile

	peerList   []*PeerData
	peerMap    map[*net.TCPAddr]*PeerData
	serviceMap *ServiceMap
	costMap    *CostMap
	selfIndex  uint
	selfPeer   bool
}

type PeerData struct {
	Index uint
	Addr  *net.TCPAddr
	Range SplitRange
}

type SplitRange struct {
	Lo SplitKey
	Hi SplitKey
}

type SplitKey struct {
	ServiceName ServiceName
	ShardID     ShardID
}

func (ref *Ref) Init(file *GlobalConfigFile, zkConn *zk.Conn, etcd *v3.Client) {
	if file == nil {
		panic(errors.New("*GlobalConfigFile is nil"))
	}

	ref.cfg = file
	ref.zkConn = zkConn
	ref.etcd = etcd
	ref.cv = sync.NewCond(&ref.mu)
	ref.peerConnMap = make(map[*net.TCPAddr]*PeerConnData, 4)
	ref.peerConnMap[ref.cfg.GRPCAddr] = nil
	ref.sendDone = true
	ref.recvDone = true
}

func (ref *Ref) GlobalConfigFile() *GlobalConfigFile {
	return ref.cfg
}

func (ref *Ref) Get() *Impl {
	ref.mu.Lock()
	impl := ref.live
	ref.mu.Unlock()
	return impl
}

func (ref *Ref) Load(ctx context.Context, rev int64) error {
	next, err := ref.loadImpl(ctx, rev)
	if err != nil {
		return err
	}

	ref.mu.Lock()
	ref.lockedAwaitReadyToFlip()
	ref.next = next
	ref.sendDone = false
	ref.recvDone = false
	go ref.prepareThreadSend()
	go ref.prepareThreadRecv()
	ref.mu.Unlock()

	return nil
}

func (ref *Ref) Flip() {
	ref.mu.Lock()
	ref.lockedAwaitReadyToFlip()
	if ref.next != nil {
		ref.live = ref.next
		ref.next = nil
	}
	ref.mu.Unlock()
}

func (ref *Ref) TakeClientConn(ctx context.Context, addr *net.TCPAddr) (*grpc.ClientConn, error) {
	if ctx == nil {
		panic(errors.New("context.Context is nil"))
	}

	addr = misc.CanonicalizeTCPAddr(addr)

	ref.mu.Lock()
	defer ref.mu.Unlock()

	data, exists := ref.peerConnMap[addr]
	if exists {
		data.refcnt++
		return data.cc, nil
	}

	dialOpts := make([]grpc.DialOption, 2)
	dialOpts[0] = roxyresolver.WithStandardResolvers(ctx)
	dialOpts[1] = grpc.WithInsecure()

	cc, err := grpc.DialContext(ctx, addr.String(), dialOpts...)
	if err != nil {
		return nil, err
	}

	data = &PeerConnData{cc: cc, refcnt: 1}
	ref.peerConnMap[addr] = data
	return cc, nil
}

func (ref *Ref) ReturnClientConn(addr *net.TCPAddr, cc *grpc.ClientConn) error {
	if cc == nil {
		panic(errors.New("*grpc.ClientConn is nil"))
	}

	addr = misc.CanonicalizeTCPAddr(addr)

	ref.mu.Lock()
	defer ref.mu.Unlock()

	data := ref.peerConnMap[addr]
	if data == nil {
		return cc.Close()
	}
	if data.cc != cc {
		panic(fmt.Errorf("data.cc %p != cc %p", data.cc, cc))
	}
	if data.refcnt <= 0 {
		panic(fmt.Errorf("data.refcnt is %d, expected > 0", data.refcnt))
	}
	data.refcnt--
	return nil
}

func (ref *Ref) Close() error {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	var errs multierror.Error

	for addr, data := range ref.peerConnMap {
		if data == nil {
			continue
		}
		if data.refcnt != 0 {
			panic(fmt.Errorf("expected 0 outstanding connections to %v, found %d", addr, data.refcnt))
		}
		if err := data.cc.Close(); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}

	if err := ref.etcd.Close(); err != nil {
		errs.Errors = append(errs.Errors, err)
	}

	return misc.ErrorOrNil(errs)
}

func (ref *Ref) prepareThreadSend() {
	// TODO: transfer data to peers

	ref.mu.Lock()
	ref.sendDone = true
	ref.cv.Signal()
	ref.mu.Unlock()
}

func (ref *Ref) prepareThreadRecv() {
	// TODO: block until all peers transfer data to us

	ref.mu.Lock()
	ref.recvDone = true
	ref.cv.Signal()
	ref.mu.Unlock()
}

func (ref *Ref) lockedReadyToFlip() bool {
	return ref.sendDone && ref.recvDone
}

func (ref *Ref) lockedAwaitReadyToFlip() {
	for !ref.lockedReadyToFlip() {
		ref.cv.Wait()
	}
}

func (ref *Ref) loadImpl(ctx context.Context, rev int64) (*Impl, error) {
	impl := &Impl{ref: ref}

	var err error
	impl.cfgPeers, err = ref.cfg.LoadPeersFile(ctx, ref.zkConn, ref.etcd, rev)
	if err != nil {
		return nil, err
	}

	impl.cfgServices, err = ref.cfg.LoadServicesFile(ctx, ref.zkConn, ref.etcd, rev)
	if err != nil {
		return nil, err
	}

	impl.cfgCost, err = ref.cfg.LoadCostFile(ctx, ref.zkConn, ref.etcd, rev)
	if err != nil {
		return nil, err
	}

	impl.selfPeer, impl.selfIndex, err = impl.processPeersFile()
	if err != nil {
		return nil, err
	}

	err = impl.processServicesFile()
	if err != nil {
		return nil, err
	}

	err = impl.processCostFile()
	if err != nil {
		return nil, err
	}

	err = impl.doComputeRanges()
	if err != nil {
		return nil, err
	}

	return impl, nil
}

func (impl *Impl) Peers() []*PeerData {
	return impl.peerList
}

func (impl *Impl) PeerByIndex(index uint) (data *PeerData, found bool) {
	length := uint(len(impl.peerList))
	if index < length {
		data = impl.peerList[index]
		found = true
	}
	return
}

func (impl *Impl) PeerByAddr(addr *net.TCPAddr) (data *PeerData, found bool) {
	addr = misc.CanonicalizeTCPAddr(addr)
	data, found = impl.peerMap[addr]
	return
}

func (impl *Impl) PeerBySplitKey(key SplitKey) (data *PeerData, found bool) {
	length := uint(len(impl.peerList))
	for index := uint(0); index < length; index++ {
		data = impl.peerList[index]
		if data.Range.Contains(key) {
			found = true
			return
		}
	}
	data = nil
	return
}

func (impl *Impl) ServiceMap() *ServiceMap {
	return impl.serviceMap
}

func (impl *Impl) CostMap() *CostMap {
	return impl.costMap
}

func (impl *Impl) processPeersFile() (bool, uint, error) {
	length := uint(len(impl.cfgPeers))

	if length == 0 {
		return false, 0, fmt.Errorf("not enough ATC servers: got 0, min 1")
	}

	if length > MaxNumATCs {
		return false, 0, fmt.Errorf("too many ATC servers: got %d, max %d", length, MaxNumATCs)
	}

	impl.peerList = make([]*PeerData, length)
	impl.peerMap = make(map[*net.TCPAddr]*PeerData, length)
	var selfPeer bool
	var selfIndex uint = length
	for index := uint(0); index < length; index++ {
		str := impl.cfgPeers[index]

		tcpAddr, err := misc.ParseTCPAddr(str, constants.PortATC)
		if err != nil {
			return false, 0, err
		}

		tcpAddr = misc.CanonicalizeTCPAddr(tcpAddr)

		if tcpAddr == impl.ref.cfg.GRPCAddr {
			selfPeer = true
			selfIndex = index
		}

		data := &PeerData{Index: index, Addr: tcpAddr}

		impl.peerList[index] = data
		impl.peerMap[tcpAddr] = data
	}

	return selfPeer, selfIndex, nil
}

func (impl *Impl) processServicesFile() error {
	var err error
	impl.serviceMap, err = NewServiceMap(impl.cfgServices)
	if err != nil {
		return err
	}

	return nil
}

func (impl *Impl) processCostFile() error {
	var err error
	impl.costMap, err = NewCostMap(impl.cfgCost)
	if err != nil {
		return err
	}

	return nil
}

func (impl *Impl) doComputeRanges() error {
	atcLength := uint(len(impl.peerList))
	totalCost := impl.serviceMap.TotalExpectedBalancerCost()
	costPerServer := totalCost / float64(atcLength)

	var (
		currIndex uint
		currCost  float64
		currHasLo bool
	)

	impl.serviceMap.Enumerate(func(sn ServiceName, id ShardID, data *ServiceData) {
		thisKey := SplitKey{sn, id}
		nextKey := thisKey.Next()

		currData := impl.peerList[currIndex]

		thisCost := data.ExpectedBalancerCostPerShard()
		currCost += thisCost

		if !currHasLo {
			if currIndex != 0 {
				prevData := impl.peerList[currIndex-1]
				prevData.Range.Hi = thisKey
			}
			currData.Range.Lo = thisKey
			currHasLo = true
		}

		currData.Range.Hi = nextKey

		if currCost < costPerServer {
			return
		}

		currIndex++
		if currIndex >= atcLength {
			currIndex--
			return
		}

		currCost = 0.0
		currHasLo = false
	})

	var currKey SplitKey
	if !currHasLo && currIndex == 0 {
		currKey = SplitKey{"", 0}
	} else if !currHasLo {
		currKey = impl.peerList[currIndex-1].Range.Hi
	} else {
		currKey = impl.peerList[currIndex].Range.Hi
		currIndex++
	}
	for currIndex < atcLength {
		currData := impl.peerList[currIndex]
		currData.Range.Lo = currKey
		currKey = currKey.Next()
		currData.Range.Hi = currKey
		currIndex++
	}
	impl.peerList[0].Range.Lo = SplitKey{"", 0}
	impl.peerList[atcLength-1].Range.Hi = SplitKey{"\x7f", 0}
	return nil
}

func (peer *PeerData) GoAway() *roxy_v0.GoAway {
	return &roxy_v0.GoAway{
		Ip:   []byte(peer.Addr.IP),
		Zone: peer.Addr.Zone,
		Port: uint32(peer.Addr.Port),
	}
}

func (r SplitRange) Contains(key SplitKey) bool {
	if cmp := CompareSplitKeys(key, r.Lo); cmp < 0 {
		return false
	}
	if cmp := CompareSplitKeys(key, r.Hi); cmp >= 0 {
		return false
	}
	return true
}

func (key SplitKey) Next() SplitKey {
	key.ShardID++
	return key
}

func CompareSplitKeys(a, b SplitKey) int {
	switch {
	case a.ServiceName < b.ServiceName:
		return -1
	case a.ServiceName > b.ServiceName:
		return 1
	case a.ShardID < b.ShardID:
		return -1
	case a.ShardID > b.ShardID:
		return 1
	default:
		return 0
	}
}
