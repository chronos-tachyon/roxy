package main

import (
	"github.com/chronos-tachyon/roxy/lib/certnames"
)

type RootFile struct {
	MainFile string `json:"mainFile"`
	CostFile string `json:"costFile"`
}

type MainFile struct {
	Servers  []string                 `json:"servers"`
	Services map[string]ServiceConfig `json:"services"`
}

type ServiceConfig struct {
	AllowedClientNames         certnames.CertNames `json:"allowedClientNames"`
	AllowedServerNames         certnames.CertNames `json:"allowedServerNames"`
	ExpectedNumClientsPerShard uint32              `json:"expectedNumClientsPerShard"`
	ExpectedNumServersPerShard uint32              `json:"expectedNumServersPerShard"`
	IsSharded                  bool                `json:"isSharded"`
	NumShards                  uint32              `json:"numShards"`
	AvgSuppliedCPSPerServer    float64             `json:"avgSuppliedCPSPerServer"`
	AvgDemandedCPQ             float64             `json:"avgDemandedCPQ"`
}

type CostFile []CostConfig

type CostConfig struct {
	A    string  `json:"a"`
	B    string  `json:"b"`
	Cost float32 `json:"cost"`
}
