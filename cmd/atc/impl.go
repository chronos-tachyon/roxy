package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	v3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/roxypb"
)

const MaxNumATCs = uint(1 << 16)

type Ref struct {
	rootPath string
	selfAddr *net.TCPAddr
	etcd     *v3.Client

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

	cfgRoot RootFile
	cfgMain MainFile
	cfgCost CostFile

	peerList   []*PeerData
	peerMap    map[*net.TCPAddr]*PeerData
	serviceMap *ServiceMap
	costMap    *CostMap
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

func (ref *Ref) Init(rootPath string, selfAddr *net.TCPAddr, etcd *v3.Client) {
	selfAddr = misc.CanonicalizeTCPAddr(selfAddr)

	if etcd == nil {
		panic(errors.New("*v3.Client is nil"))
	}

	ref.rootPath = rootPath
	ref.selfAddr = selfAddr
	ref.etcd = etcd
	ref.cv = sync.NewCond(&ref.mu)
	ref.peerConnMap = make(map[*net.TCPAddr]*PeerConnData, 4)
	ref.peerConnMap[selfAddr] = nil
	ref.sendDone = true
	ref.recvDone = true
}

func (ref *Ref) RootConfigFilePath() string {
	return ref.rootPath
}

func (ref *Ref) SelfAddr() *net.TCPAddr {
	return ref.selfAddr
}

func (ref *Ref) Get() *Impl {
	ref.mu.Lock()
	impl := ref.live
	ref.mu.Unlock()
	return impl
}

func (ref *Ref) Load(ctx context.Context) error {
	next, err := ref.doLoadImpl(ctx)
	if err != nil {
		return err
	}
	ref.Prepare(next)
	return nil
}

func (ref *Ref) Prepare(next *Impl) {
	if next == nil {
		panic(errors.New("*Impl is nil"))
	}

	ref.mu.Lock()
	ref.lockedAwaitReadyToFlip()
	ref.next = next
	ref.sendDone = false
	ref.recvDone = false
	go ref.prepareThreadSend()
	go ref.prepareThreadRecv()
	ref.mu.Unlock()
}

func (ref *Ref) Flip() (prev *Impl) {
	ref.mu.Lock()
	ref.lockedAwaitReadyToFlip()
	prev = ref.live
	ref.live = ref.next
	ref.next = nil
	ref.mu.Unlock()
	return prev
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

	err := errs.ErrorOrNil()
	return err
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

func (ref *Ref) doLoadImpl(ctx context.Context) (*Impl, error) {
	impl := new(Impl)

	impl.ref = ref

	err := impl.doLoadRootFile()
	if err != nil {
		return nil, err
	}

	err = impl.doLoadMainFile()
	if err != nil {
		return nil, err
	}

	err = impl.doLoadCostFile()
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

func (impl *Impl) doLoadRootFile() error {
	raw, err := ioutil.ReadFile(impl.ref.rootPath)
	if err != nil {
		return err
	}

	err = misc.StrictUnmarshalJSON(raw, &impl.cfgRoot)
	if err != nil {
		return err
	}

	if impl.cfgRoot.MainFile == "" {
		impl.cfgRoot.MainFile = "/etc/opt/atc/main.json"
	}
	if impl.cfgRoot.CostFile == "" {
		impl.cfgRoot.CostFile = "/etc/opt/atc/cost.json"
	}

	return nil
}

func (impl *Impl) doLoadMainFile() error {
	raw, err := ioutil.ReadFile(impl.cfgRoot.MainFile)
	if err != nil {
		return err
	}

	err = misc.StrictUnmarshalJSON(raw, &impl.cfgMain)
	if err != nil {
		return err
	}

	atcLength := uint(len(impl.cfgMain.Servers))
	if atcLength > MaxNumATCs {
		return fmt.Errorf("too many ATC servers: got %d, max %d", atcLength, MaxNumATCs)
	}

	impl.serviceMap, err = NewServiceMap(impl.cfgMain)
	if err != nil {
		return err
	}

	impl.peerList = make([]*PeerData, atcLength)
	impl.peerMap = make(map[*net.TCPAddr]*PeerData, atcLength)
	foundMatch := false
	for index := uint(0); index < atcLength; index++ {
		str := impl.cfgMain.Servers[index]

		tcpAddr, err := misc.ParseTCPAddr(str, "2987")
		if err != nil {
			return err
		}

		tcpAddr = misc.CanonicalizeTCPAddr(tcpAddr)

		if tcpAddr == impl.ref.selfAddr {
			foundMatch = true
		}

		data := &PeerData{Index: index, Addr: tcpAddr}

		impl.peerList[index] = data
		impl.peerMap[tcpAddr] = data
	}
	if !foundMatch {
		return fmt.Errorf("our TCP address (%v) is not listed in %q", impl.ref.selfAddr, impl.cfgRoot.MainFile)
	}

	return nil
}

func (impl *Impl) doLoadCostFile() error {
	raw, err := ioutil.ReadFile(impl.cfgRoot.CostFile)
	if err != nil {
		return err
	}
	err = misc.StrictUnmarshalJSON(raw, &impl.cfgCost)
	if err != nil {
		return err
	}
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

func (peer *PeerData) GoAway() *roxypb.GoAway {
	return &roxypb.GoAway{
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
