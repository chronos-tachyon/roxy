package main

import (
	"github.com/chronos-tachyon/roxy/lib/certnames"
)

type ServiceData struct {
	AllowedClientNames         certnames.CertNames
	AllowedServerNames         certnames.CertNames
	AvgSuppliedCPSPerServer    float64
	AvgDemandedCPQ             float64
	ExpectedNumClientsPerShard uint32
	ExpectedNumServersPerShard uint32
	NumShards                  uint32
	IsSharded                  bool
}

func (svc *ServiceData) EffectiveNumShards() uint32 {
	var numShards uint32 = 1
	if svc.IsSharded {
		numShards = svc.NumShards
	}
	return numShards
}

func (svc *ServiceData) ExpectedStats() Stats {
	return svc.
		ExpectedStatsPerShard().
		Scale(uint(svc.EffectiveNumShards()))
}

func (svc *ServiceData) ExpectedStatsPerShard() Stats {
	const BaseCost = 8
	const ClientCostFactor = 1
	const ServerCostFactor = 1
	nc := svc.ExpectedNumClientsPerShard
	ns := svc.ExpectedNumServersPerShard
	uc, fc := uint(nc), float64(nc)
	us, fs := uint(ns), float64(ns)
	return Stats{
		NumShards:  1,
		NumClients: uc,
		NumServers: us,
		PeerCost:   BaseCost + ClientCostFactor*fc + ServerCostFactor*fs,
	}
}
