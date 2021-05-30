package main

import (
	"net"
	"time"

	"github.com/chronos-tachyon/roxy/lib/costhistory"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ServerData struct {
	ShardData   *ShardData
	Unique      string
	Location    Location
	ServerName  string
	Addr        *net.TCPAddr
	DeclaredCPS float64
	MeasuredCPS float64
	IsAlive     bool
	IsServing   bool
	GoAwayCh    chan *roxy_v0.GoAway
	ExpireTime  time.Time
	CostHistory costhistory.CostHistory
}

func (serverData *ServerData) LockedSendGoAway(goAway *roxy_v0.GoAway) {
	if serverData.IsAlive {
		select {
		case serverData.GoAwayCh <- goAway:
		default:
		}
	}
}

func (serverData *ServerData) LockedUpdate(isAlive bool, isServing bool) {
	histData := serverData.CostHistory.Data()
	now := histData.Now
	measuredCPS := histData.PerSecond

	oldIsAlive := serverData.IsAlive
	oldIsServing := serverData.IsServing
	oldMeasuredCPS := serverData.MeasuredCPS

	if oldIsAlive && !isAlive {
		close(serverData.GoAwayCh)
		serverData.ExpireTime = now.Add(ExpireInterval)
	}

	if oldIsServing {
		serverData.ShardData.DeclaredSupplyCPS -= serverData.DeclaredCPS
		serverData.ShardData.MeasuredSupplyCPS -= oldMeasuredCPS
	}
	serverData.IsAlive = isAlive
	serverData.IsServing = isServing
	serverData.MeasuredCPS = measuredCPS
	if isServing {
		serverData.ShardData.DeclaredSupplyCPS += serverData.DeclaredCPS
		serverData.ShardData.MeasuredSupplyCPS += measuredCPS
	}

	if isAlive && !oldIsAlive {
		serverData.ExpireTime = time.Time{}
		serverData.GoAwayCh = make(chan *roxy_v0.GoAway, 1)
	}
}

func (serverData *ServerData) LockedIsExpired() bool {
	t := serverData.ExpireTime
	if t.IsZero() {
		return false
	}
	now := serverData.CostHistory.Data().Now
	return now.Sub(t) >= 0
}

func (serverData *ServerData) LockedPeriodic() {
	serverData.CostHistory.Update()
	histData := serverData.CostHistory.Data()
	oldCPS := serverData.MeasuredCPS
	newCPS := histData.PerSecond
	serverData.MeasuredCPS = newCPS
	if serverData.IsServing {
		serverData.ShardData.MeasuredSupplyCPS += (newCPS - oldCPS)
		t := serverData.ExpireTime
		now := histData.Now
		if !t.IsZero() && now.Sub(t) >= 0 {
			serverData.IsServing = false
			serverData.ShardData.DeclaredDemandCPS -= serverData.DeclaredCPS
			serverData.ShardData.MeasuredDemandCPS -= serverData.MeasuredCPS
		}
	}
}

func (serverData *ServerData) LockedToProto() *roxy_v0.ServerData {
	return &roxy_v0.ServerData{
		ServiceName:           string(serverData.ShardData.ServiceName),
		ShardId:               uint32(serverData.ShardData.ShardID),
		HasShardId:            serverData.ShardData.HasShardID,
		Unique:                serverData.Unique,
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
