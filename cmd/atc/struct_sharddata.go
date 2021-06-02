package main

import (
	"net"
	"sync"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ShardData struct {
	Key               Key
	Mutex             sync.Mutex
	Cond              *sync.Cond
	ServiceData       *ServiceData
	CostMap           *CostMap
	Clients           map[string]*ClientData
	Servers           map[string]*ServerData
	DeclaredDemandCPS float64
	MeasuredDemandCPS float64
	DeclaredSupplyCPS float64
	MeasuredSupplyCPS float64
	AssignedCPS       float64
}

func (shardData *ShardData) Client(uniqueID string) *ClientData {
	shardData.Mutex.Lock()
	clientData := shardData.Clients[uniqueID]
	shardData.Mutex.Unlock()
	return clientData
}

func (shardData *ShardData) Server(uniqueID string) *ServerData {
	shardData.Mutex.Lock()
	serverData := shardData.Servers[uniqueID]
	shardData.Mutex.Unlock()
	return serverData
}

func (shardData *ShardData) GetOrInsertClient(first *roxy_v0.ClientData) *ClientData {
	shardData.Mutex.Lock()
	clientData := shardData.LockedGetOrInsertClient(first)
	shardData.Mutex.Unlock()
	return clientData
}

func (shardData *ShardData) GetOrInsertServer(first *roxy_v0.ServerData, tcpAddr *net.TCPAddr) *ServerData {
	shardData.Mutex.Lock()
	serverData := shardData.LockedGetOrInsertServer(first, tcpAddr)
	shardData.Mutex.Unlock()
	return serverData
}

func (shardData *ShardData) LockedGetOrInsertClient(first *roxy_v0.ClientData) *ClientData {
	clientData := shardData.Clients[first.UniqueId]
	if clientData == nil {
		clientData = &ClientData{
			ShardData:   shardData,
			UniqueID:    first.UniqueId,
			Assignments: make(map[string]float64, shardData.ServiceData.ExpectedNumServersPerShard),
		}
		clientData.CostHistory.Init()
		shardData.Clients[first.UniqueId] = clientData
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

func (shardData *ShardData) LockedGetOrInsertServer(first *roxy_v0.ServerData, tcpAddr *net.TCPAddr) *ServerData {
	tcpAddr = misc.CanonicalizeTCPAddr(tcpAddr)

	serverData := shardData.Servers[first.UniqueId]
	if serverData == nil {
		serverData = &ServerData{
			ShardData:   shardData,
			UniqueID:    first.UniqueId,
			Assignments: make(map[string]float64, shardData.ServiceData.ExpectedNumClientsPerShard),
		}
		serverData.CostHistory.Init()
		shardData.Servers[first.UniqueId] = serverData
	}
	if serverData.IsServing {
		oldCPS := serverData.DeclaredCPS
		newCPS := first.DeclaredCostPerSecond
		shardData.DeclaredSupplyCPS += (newCPS - oldCPS)
	}
	serverData.Location = Location(first.Location)
	serverData.ServerName = first.ServerName
	serverData.Addr = tcpAddr
	serverData.DeclaredCPS = first.DeclaredCostPerSecond
	return serverData
}

func (shardData *ShardData) LockedPeriodic() {
	for clientID, clientData := range shardData.Clients {
		clientData.LockedPeriodic()
		if clientData.LockedIsExpired() {
			clientData.LockedDelete()
			delete(shardData.Clients, clientID)
		}
	}

	for serverID, serverData := range shardData.Servers {
		serverData.LockedPeriodic()
		if serverData.LockedIsExpired() {
			serverData.LockedDelete()
			delete(shardData.Servers, serverID)
		}
	}

	shardData.LockedRebalance()
}

func (shardData *ShardData) LockedRebalance() {
}

func (shardData *ShardData) ToProto() *roxy_v0.ShardData {
	shardData.Mutex.Lock()
	out := shardData.LockedToProto()
	shardData.Mutex.Unlock()
	return out
}

func (shardData *ShardData) LockedToProto() *roxy_v0.ShardData {
	out := &roxy_v0.ShardData{
		ServiceName:                 string(shardData.Key.ServiceName),
		ShardNumber:                 uint32(shardData.Key.ShardNumber),
		HasShardNumber:              shardData.Key.HasShardNumber,
		DeclaredDemandCostPerSecond: shardData.DeclaredDemandCPS,
		MeasuredDemandCostPerSecond: shardData.MeasuredDemandCPS,
		DeclaredSupplyCostPerSecond: shardData.DeclaredSupplyCPS,
		MeasuredSupplyCostPerSecond: shardData.MeasuredSupplyCPS,
	}
	for _, clientData := range shardData.Clients {
		if clientData.IsAlive {
			out.NumClients++
		}
	}
	for _, serverData := range shardData.Servers {
		if serverData.IsAlive {
			out.NumServers++
		}
	}
	return out
}
