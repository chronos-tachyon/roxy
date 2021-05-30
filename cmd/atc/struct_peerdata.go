package main

import (
	"net"

	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type PeerData struct {
	Index uint
	Addr  *net.TCPAddr
	Range Range
}

func (peerData *PeerData) Contains(key Key) bool {
	return (peerData != nil && peerData.Range.Contains(key))
}

func (peerData *PeerData) GoAway() *roxy_v0.GoAway {
	return &roxy_v0.GoAway{
		Ip:   []byte(peerData.Addr.IP),
		Zone: peerData.Addr.Zone,
		Port: uint32(peerData.Addr.Port),
	}
}
