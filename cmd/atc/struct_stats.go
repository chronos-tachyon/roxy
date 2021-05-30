package main

type Stats struct {
	NumShards  uint
	NumClients uint
	NumServers uint
	PeerCost   float64
}

func (stats Stats) Add(other Stats) Stats {
	stats.NumShards += other.NumShards
	stats.NumClients += other.NumClients
	stats.NumServers += other.NumServers
	stats.PeerCost += other.PeerCost
	return stats
}

func (stats Stats) Scale(k uint) Stats {
	stats.NumShards *= k
	stats.NumClients *= k
	stats.NumServers *= k
	stats.PeerCost *= float64(k)
	return stats
}
