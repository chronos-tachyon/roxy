package main

import (
	"net"
	"sync"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/syncrand"
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
	PeriodicCounter   uint64
}

func (shardData *ShardData) NumServersLocked() (minServers, maxServers uint) {
	for _, serverData := range shardData.Servers {
		if serverData.IsServing {
			maxServers++
		}
	}

	minServers = maxServers
	if minServers > 3 {
		minServers = 3
	}

	return
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
	clientData := shardData.GetOrInsertClientLocked(first)
	shardData.Mutex.Unlock()
	return clientData
}

func (shardData *ShardData) GetOrInsertServer(first *roxy_v0.ServerData, tcpAddr *net.TCPAddr) *ServerData {
	shardData.Mutex.Lock()
	serverData := shardData.GetOrInsertServerLocked(first, tcpAddr)
	shardData.Mutex.Unlock()
	return serverData
}

func (shardData *ShardData) GetOrInsertClientLocked(first *roxy_v0.ClientData) *ClientData {
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

func (shardData *ShardData) GetOrInsertServerLocked(first *roxy_v0.ServerData, tcpAddr *net.TCPAddr) *ServerData {
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

func (shardData *ShardData) PeriodicLocked() {
	shardData.PeriodicCounter++
	counter := shardData.PeriodicCounter

	shouldShedExcessServers := (counter & 0x3f) == 0x00
	shouldRebalance := (counter & 0x0f) == 0x00

	for serverID, serverData := range shardData.Servers {
		serverData.PeriodicLocked()
		if serverData.IsExpiredLocked() {
			serverData.DeleteLocked()
			delete(shardData.Servers, serverID)
		}
	}

	for clientID, clientData := range shardData.Clients {
		clientData.PeriodicLocked()
		if clientData.IsExpiredLocked() {
			clientData.DeleteLocked()
			delete(shardData.Clients, clientID)
		} else if !clientData.IsOKLocked() {
			clientData.RebalanceLocked(false)
		} else if shouldShedExcessServers {
			clientData.ShedExcessServersLocked()
		}
	}

	if shouldRebalance {
		clientList := make([]*ClientData, 0, len(shardData.Clients))
		for _, clientData := range shardData.Clients {
			if clientData.IsServing {
				clientList = append(clientList, clientData)
			}
		}

		if len(clientList) != 0 {
			rng := syncrand.Global()
			index := rng.Intn(len(clientList))
			clientData := clientList[index]
			clientData.RebalanceLocked(false)
		}
	}
}

func (shardData *ShardData) ToProto() *roxy_v0.ShardData {
	shardData.Mutex.Lock()
	out := shardData.ToProtoLocked()
	shardData.Mutex.Unlock()
	return out
}

func (shardData *ShardData) ToProtoLocked() *roxy_v0.ShardData {
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
