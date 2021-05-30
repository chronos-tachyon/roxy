package main

import (
	"net"
	"sync"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ShardData struct {
	ServiceName       ServiceName
	ShardID           ShardID
	HasShardID        bool
	Mutex             sync.Mutex
	Cond              *sync.Cond
	ServiceData       *ServiceData
	ClientsByUnique   map[string]*ClientData
	ServersByUnique   map[string]*ServerData
	DeclaredDemandCPS float64
	MeasuredDemandCPS float64
	DeclaredSupplyCPS float64
	MeasuredSupplyCPS float64
}

func (shardData *ShardData) Key() Key {
	return Key{shardData.ServiceName, shardData.ShardID}
}

func (shardData *ShardData) Client(unique string) *ClientData {
	shardData.Mutex.Lock()
	clientData := shardData.ClientsByUnique[unique]
	shardData.Mutex.Unlock()
	return clientData
}

func (shardData *ShardData) Server(unique string) *ServerData {
	shardData.Mutex.Lock()
	serverData := shardData.ServersByUnique[unique]
	shardData.Mutex.Unlock()
	return serverData
}

func (shardData *ShardData) GetOrInsertClient(first *roxy_v0.ClientData) *ClientData {
	shardData.Mutex.Lock()
	clientData := shardData.LockedGetOrInsertClient(first)
	shardData.Mutex.Unlock()
	return clientData
}

func (shardData *ShardData) GetOrInsertServer(first *roxy_v0.ServerData) *ServerData {
	shardData.Mutex.Lock()
	serverData := shardData.LockedGetOrInsertServer(first)
	shardData.Mutex.Unlock()
	return serverData
}

func (shardData *ShardData) LockedGetOrInsertClient(first *roxy_v0.ClientData) *ClientData {
	clientData := shardData.ClientsByUnique[first.Unique]
	if clientData == nil {
		clientData = &ClientData{
			ShardData: shardData,
			Unique:    first.Unique,
		}
		clientData.CostHistory.Init()
		shardData.ClientsByUnique[first.Unique] = clientData
	}
	if clientData.IsServing {
		oldCPS := clientData.DeclaredCPS
		newCPS := first.DeclaredCostPerSecond
		shardData.DeclaredDemandCPS += (newCPS - oldCPS)
	}
	clientData.Location = Location(first.Location)
	clientData.DeclaredCPS = first.DeclaredCostPerSecond
	return clientData
}

func (shardData *ShardData) LockedGetOrInsertServer(first *roxy_v0.ServerData) *ServerData {
	addr := &net.TCPAddr{
		IP:   net.IP(first.Ip),
		Port: int(first.Port),
		Zone: first.Zone,
	}
	addr = misc.CanonicalizeTCPAddr(addr)

	serverData := shardData.ServersByUnique[first.Unique]
	if serverData == nil {
		serverData = &ServerData{
			ShardData: shardData,
			Unique:    first.Unique,
		}
		serverData.CostHistory.Init()
		shardData.ServersByUnique[first.Unique] = serverData
	}
	if serverData.IsServing {
		oldCPS := serverData.DeclaredCPS
		newCPS := first.DeclaredCostPerSecond
		shardData.DeclaredSupplyCPS += (newCPS - oldCPS)
	}
	serverData.Location = Location(first.Location)
	serverData.ServerName = first.ServerName
	serverData.Addr = addr
	serverData.DeclaredCPS = first.DeclaredCostPerSecond
	return serverData
}

func (shardData *ShardData) LockedPeriodic() {
	for unique, clientData := range shardData.ClientsByUnique {
		clientData.LockedPeriodic()
		if clientData.LockedIsExpired() {
			delete(shardData.ClientsByUnique, unique)
		}
	}
	for unique, serverData := range shardData.ServersByUnique {
		serverData.LockedPeriodic()
		if serverData.LockedIsExpired() {
			delete(shardData.ServersByUnique, unique)
		}
	}
}

func (shardData *ShardData) ToProto() *roxy_v0.ShardData {
	shardData.Mutex.Lock()
	out := shardData.LockedToProto()
	shardData.Mutex.Unlock()
	return out
}

func (shardData *ShardData) LockedToProto() *roxy_v0.ShardData {
	out := &roxy_v0.ShardData{
		ServiceName:                 string(shardData.ServiceName),
		ShardId:                     uint32(shardData.ShardID),
		HasShardId:                  shardData.HasShardID,
		DeclaredDemandCostPerSecond: shardData.DeclaredDemandCPS,
		MeasuredDemandCostPerSecond: shardData.MeasuredDemandCPS,
		DeclaredSupplyCostPerSecond: shardData.DeclaredSupplyCPS,
		MeasuredSupplyCostPerSecond: shardData.MeasuredSupplyCPS,
	}
	for _, clientData := range shardData.ClientsByUnique {
		if clientData.IsAlive {
			out.NumClients++
		}
	}
	for _, serverData := range shardData.ServersByUnique {
		if serverData.IsAlive {
			out.NumServers++
		}
	}
	return out
}
