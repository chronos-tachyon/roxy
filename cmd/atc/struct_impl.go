package main

import (
	"context"
	"fmt"
	"net"

	"github.com/go-zookeeper/zk"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
)

const MaxNumATCs = uint(1 << 16)

type Impl struct {
	GlobalConfig   *GlobalConfigFile
	PeersConfig    PeersFile
	ServicesConfig ServicesFile
	CostConfig     CostFile
	ServiceMap     *ServiceMap
	CostMap        *CostMap
	PeerList       []*PeerData
	PeerMap        map[*net.TCPAddr]*PeerData
	SelfIndex      uint
	SelfPeer       bool
	SelfData       *PeerData
	SelfStats      Stats
}

func (impl *Impl) PeerByAddr(addr *net.TCPAddr) *PeerData {
	addr = misc.CanonicalizeTCPAddr(addr)
	peerData := impl.PeerMap[addr]
	return peerData
}

func (impl *Impl) PeerByKey(key Key) *PeerData {
	length := uint(len(impl.PeerList))
	for index := uint(0); index < length; index++ {
		peerData := impl.PeerList[index]
		if peerData.Range.Contains(key) {
			return peerData
		}
	}
	return nil
}

func (impl *Impl) doLoad(ctx context.Context, zkConn *zk.Conn, etcd *v3.Client, rev int64) error {
	var err error

	impl.PeersConfig, err = impl.GlobalConfig.LoadPeersFile(ctx, zkConn, etcd, rev)
	if err != nil {
		return err
	}

	impl.ServicesConfig, err = impl.GlobalConfig.LoadServicesFile(ctx, zkConn, etcd, rev)
	if err != nil {
		return err
	}

	impl.CostConfig, err = impl.GlobalConfig.LoadCostFile(ctx, zkConn, etcd, rev)
	if err != nil {
		return err
	}

	err = impl.processPeersFile()
	if err != nil {
		return err
	}

	err = impl.processServicesFile()
	if err != nil {
		return err
	}

	err = impl.processCostFile()
	if err != nil {
		return err
	}

	err = impl.doComputeRanges()
	if err != nil {
		return err
	}

	return nil
}

func (impl *Impl) processPeersFile() error {
	impl.SelfPeer = false
	impl.SelfIndex = 0

	length := uint(len(impl.PeersConfig))
	if length == 0 {
		return fmt.Errorf("not enough ATC servers: got 0, min 1")
	}
	if length > MaxNumATCs {
		return fmt.Errorf("too many ATC servers: got %d, max %d", length, MaxNumATCs)
	}

	impl.PeerList = make([]*PeerData, length)
	impl.PeerMap = make(map[*net.TCPAddr]*PeerData, length)
	var selfPeer bool
	var selfIndex uint = length
	for index := uint(0); index < length; index++ {
		str := impl.PeersConfig[index]

		tcpAddr, err := misc.ParseTCPAddr(str, constants.PortATC)
		if err != nil {
			return err
		}

		tcpAddr = misc.CanonicalizeTCPAddr(tcpAddr)

		if tcpAddr == impl.GlobalConfig.GRPCAddr {
			selfPeer = true
			selfIndex = index
		}

		data := &PeerData{Index: index, Addr: tcpAddr}

		impl.PeerList[index] = data
		impl.PeerMap[tcpAddr] = data
	}

	impl.SelfPeer = selfPeer
	impl.SelfIndex = selfIndex
	return nil
}

func (impl *Impl) processServicesFile() error {
	var err error
	impl.ServiceMap, err = NewServiceMap(impl.ServicesConfig)
	if err != nil {
		return err
	}

	return nil
}

func (impl *Impl) processCostFile() error {
	var err error
	impl.CostMap, err = NewCostMap(impl.CostConfig)
	if err != nil {
		return err
	}

	return nil
}

func (impl *Impl) doComputeRanges() error {
	peerLen := uint(len(impl.PeerList))
	stats := impl.ServiceMap.ExpectedStatsTotal()
	costPerPeer := stats.PeerCost / float64(peerLen)

	var (
		currIndex uint
		currCost  float64
		currHasLo bool
	)

	var lastData *ServiceData
	var lastStats Stats

	impl.ServiceMap.Enumerate(func(thisKey Key, thisData *ServiceData) {
		thisStats := lastStats
		if thisData != lastData {
			thisStats = thisData.ExpectedStatsPerShard()
			lastStats = thisStats
			lastData = thisData
		}

		thisCost := thisStats.PeerCost

		currData := impl.PeerList[currIndex]
		currCost += thisCost

		if !currHasLo {
			if currIndex != 0 {
				prevData := impl.PeerList[currIndex-1]
				prevData.Range.Hi = thisKey
			}
			currData.Range.Lo = thisKey
			currHasLo = true
		}

		currData.Range.Hi = thisKey.Next()

		if currCost < costPerPeer {
			return
		}

		currIndex++
		if currIndex >= peerLen {
			currIndex--
			return
		}

		currCost = 0.0
		currHasLo = false
	})

	var currKey Key
	if !currHasLo && currIndex == 0 {
		currKey = Key{"", 0, false}
	} else if !currHasLo {
		currKey = impl.PeerList[currIndex-1].Range.Hi
	} else {
		currKey = impl.PeerList[currIndex].Range.Hi
		currIndex++
	}
	for currIndex < peerLen {
		currData := impl.PeerList[currIndex]
		currData.Range.Lo = currKey
		currKey = currKey.Next()
		currData.Range.Hi = currKey
		currIndex++
	}

	if impl.SelfPeer {
		impl.SelfData = impl.PeerList[impl.SelfIndex]
		impl.SelfStats = impl.ServiceMap.ExpectedStatsByRange(impl.SelfData.Range)
	}

	return nil
}
