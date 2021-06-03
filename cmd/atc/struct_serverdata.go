package main

import (
	"math"
	"net"
	"time"

	"github.com/chronos-tachyon/roxy/lib/costhistory"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ServerData struct {
	ShardData   *ShardData
	UniqueID    string
	Location    Location
	ServerName  string
	Addr        *net.TCPAddr
	DeclaredCPS float64
	MeasuredCPS float64
	AssignedCPS float64
	IsAlive     bool
	IsServing   bool
	GoAwayCh    chan *roxy_v0.GoAway
	ExpireTime  time.Time
	CostHistory costhistory.CostHistory
	Assignments map[string]float64
}

func (serverData *ServerData) AvailableCPSLocked() float64 {
	return serverData.DeclaredCPS - serverData.AssignedCPS
}

func (serverData *ServerData) UtilizationRatioLocked() float64 {
	assignedCPS := serverData.AssignedCPS
	declaredCPS := math.Max(1.0, serverData.DeclaredCPS)
	return assignedCPS / declaredCPS
}

func (serverData *ServerData) SendGoAwayLocked(goAway *roxy_v0.GoAway) {
	if serverData.IsAlive {
		select {
		case serverData.GoAwayCh <- goAway:
		default:
		}
	}
}

func (serverData *ServerData) UpdateLocked(isAlive bool, isServing bool) {
	shardData := serverData.ShardData
	histData := serverData.CostHistory.Data()
	now := histData.Now

	isAliveChanged := (serverData.IsAlive != isAlive)
	serverData.IsAlive = isAlive
	if isAliveChanged && !isAlive {
		close(serverData.GoAwayCh)
		serverData.ExpireTime = now.Add(ExpireInterval)
	}
	if isAliveChanged && isAlive {
		serverData.ExpireTime = time.Time{}
		serverData.GoAwayCh = make(chan *roxy_v0.GoAway, 1)
	}

	isServingChanged := (serverData.IsServing != isServing)
	if serverData.IsServing {
		shardData.DeclaredSupplyCPS -= serverData.DeclaredCPS
		shardData.MeasuredSupplyCPS -= serverData.MeasuredCPS
	}
	serverData.MeasuredCPS = histData.PerSecond
	serverData.IsServing = isServing
	if serverData.IsServing {
		shardData.DeclaredSupplyCPS += serverData.DeclaredCPS
		shardData.MeasuredSupplyCPS += serverData.MeasuredCPS
	}
	if isServingChanged && !isServing {
		serverData.DeleteLocked()
	}
}

func (serverData *ServerData) DeleteLocked() {
	shardData := serverData.ShardData

	if serverData.IsServing {
		shardData.DeclaredSupplyCPS -= serverData.DeclaredCPS
		shardData.MeasuredSupplyCPS -= serverData.MeasuredCPS
		serverData.IsServing = false
	}

	for clientID := range serverData.Assignments {
		clientData := shardData.Clients[clientID]
		clientData.DeleteServerLocked(serverData)
	}
}

func (serverData *ServerData) IsExpiredLocked() bool {
	t := serverData.ExpireTime
	if t.IsZero() {
		return false
	}
	now := serverData.CostHistory.Data().Now
	return now.Sub(t) >= 0
}

func (serverData *ServerData) PeriodicLocked() {
	shardData := serverData.ShardData

	serverData.CostHistory.Update()
	histData := serverData.CostHistory.Data()

	oldCPS := serverData.MeasuredCPS
	newCPS := histData.PerSecond
	serverData.MeasuredCPS = newCPS
	if serverData.IsServing {
		shardData.MeasuredSupplyCPS += (newCPS - oldCPS)
	}
}

func (serverData *ServerData) ToProtoLocked() *roxy_v0.ServerData {
	key := serverData.ShardData.Key
	return &roxy_v0.ServerData{
		ServiceName:           string(key.ServiceName),
		ShardNumber:           uint32(key.ShardNumber),
		HasShardNumber:        key.HasShardNumber,
		UniqueId:              serverData.UniqueID,
		Location:              string(serverData.Location),
		ServerName:            serverData.ServerName,
		Ip:                    []byte(serverData.Addr.IP),
		Zone:                  serverData.Addr.Zone,
		Port:                  uint32(serverData.Addr.Port),
		DeclaredCostPerSecond: serverData.DeclaredCPS,
		MeasuredCostPerSecond: serverData.MeasuredCPS,
		IsAlive:               serverData.IsAlive,
		IsServing:             serverData.IsServing,
		History:               costhistory.SamplesToProto(serverData.CostHistory.Snapshot()),
	}
}
