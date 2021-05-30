package main

import (
	"time"

	"github.com/chronos-tachyon/roxy/lib/costhistory"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ClientData struct {
	ShardData   *ShardData
	Unique      string
	Location    Location
	DeclaredCPS float64
	MeasuredCPS float64
	IsAlive     bool
	IsServing   bool
	GoAwayCh    chan *roxy_v0.GoAway
	ExpireTime  time.Time
	CostHistory costhistory.CostHistory
}

func (clientData *ClientData) LockedSendGoAway(goAway *roxy_v0.GoAway) {
	if clientData.IsAlive {
		select {
		case clientData.GoAwayCh <- goAway:
		default:
		}
	}
}

func (clientData *ClientData) LockedUpdate(isAlive bool, isServing bool) {
	histData := clientData.CostHistory.Data()
	now := histData.Now
	measuredCPS := histData.PerSecond

	oldIsAlive := clientData.IsAlive
	oldIsServing := clientData.IsServing
	oldMeasuredCPS := clientData.MeasuredCPS

	if oldIsAlive && !isAlive {
		close(clientData.GoAwayCh)
		clientData.ExpireTime = now.Add(ExpireInterval)
	}

	if oldIsServing {
		clientData.ShardData.DeclaredDemandCPS -= clientData.DeclaredCPS
		clientData.ShardData.MeasuredDemandCPS -= oldMeasuredCPS
	}
	clientData.IsAlive = isAlive
	clientData.IsServing = isServing
	clientData.MeasuredCPS = measuredCPS
	if isServing {
		clientData.ShardData.DeclaredDemandCPS += clientData.DeclaredCPS
		clientData.ShardData.MeasuredDemandCPS += measuredCPS
	}

	if isAlive && !oldIsAlive {
		clientData.ExpireTime = time.Time{}
		clientData.GoAwayCh = make(chan *roxy_v0.GoAway, 1)
	}
}

func (clientData *ClientData) LockedIsExpired() bool {
	t := clientData.ExpireTime
	if t.IsZero() {
		return false
	}
	now := clientData.CostHistory.Data().Now
	return now.Sub(t) >= 0
}

func (clientData *ClientData) LockedPeriodic() {
	clientData.CostHistory.Update()
	histData := clientData.CostHistory.Data()
	oldCPS := clientData.MeasuredCPS
	newCPS := histData.PerSecond
	clientData.MeasuredCPS = newCPS
	if clientData.IsServing {
		clientData.ShardData.MeasuredDemandCPS += (newCPS - oldCPS)
		t := clientData.ExpireTime
		now := histData.Now
		if !t.IsZero() && now.Sub(t) >= 0 {
			clientData.IsServing = false
			clientData.ShardData.DeclaredDemandCPS -= clientData.DeclaredCPS
			clientData.ShardData.MeasuredDemandCPS -= clientData.MeasuredCPS
		}
	}
}

func (clientData *ClientData) LockedToProto() *roxy_v0.ClientData {
	return &roxy_v0.ClientData{
		ServiceName:           string(clientData.ShardData.ServiceName),
		ShardId:               uint32(clientData.ShardData.ShardID),
		HasShardId:            clientData.ShardData.HasShardID,
		Unique:                clientData.Unique,
		Location:              string(clientData.Location),
		DeclaredCostPerSecond: clientData.DeclaredCPS,
		MeasuredCostPerSecond: clientData.MeasuredCPS,
		IsAlive:               clientData.IsAlive,
		IsServing:             clientData.IsServing,
		History:               costhistory.SamplesToProto(clientData.CostHistory.Snapshot()),
	}
}
