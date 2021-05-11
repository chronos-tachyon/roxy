package main

type RootFile struct {
	MainFile string `json:"mainFile"`
	CostFile string `json:"costFile"`
}

type MainFile struct {
	Servers  []string                 `json:"servers"`
	Services map[string]ServiceConfig `json:"services"`
}

type ServiceConfig struct {
	IsSharded                  bool     `json:"isSharded"`
	NumShards                  uint32   `json:"numShards"`
	MaxLoadPerServer           float32  `json:"maxLoadPerServer"`
	ExpectedNumClientsPerShard uint32   `json:"expectedNumClientsPerShard"`
	ExpectedNumServersPerShard uint32   `json:"expectedNumServersPerShard"`
	AllowedClientCommonNames   []string `json:"allowedClientCommonNames"`
	AllowedServerCommonNames   []string `json:"allowedServerCommonNames"`
}

type CostFile []CostConfig

type CostConfig struct {
	A    string  `json:"a"`
	B    string  `json:"b"`
	Cost float32 `json:"cost"`
}
