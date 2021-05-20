package membership

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// type ServerSet {{{

// ServerSet represents a service advertisement in Finagle's ServerSet format.
//
// See: https://github.com/twitter/finagle/blob/develop/finagle-serversets/src/main/scala/com/twitter/finagle/serverset2/Entry.scala
type ServerSet struct {
	ServiceEndpoint     *ServerSetEndpoint            `json:"serviceEndpoint"`
	AdditionalEndpoints map[string]*ServerSetEndpoint `json:"additionalEndpoints"`
	Status              ServerSetStatus               `json:"status"`
	ShardID             *int32                        `json:"shard,omitempty"`
	Metadata            map[string]string             `json:"metadata,omitempty"`
}

// IsAlive returns true if this represents the advertisement of a live server.
func (ss *ServerSet) IsAlive() bool {
	if ss == nil {
		return false
	}
	return ss.Status == StatusAlive
}

// NamedPorts returns the list of named ports advertized by the server.
func (ss *ServerSet) NamedPorts() []string {
	if !ss.IsAlive() {
		return nil
	}
	list := make([]string, 0, len(ss.AdditionalEndpoints))
	for name, endpoint := range ss.AdditionalEndpoints {
		if endpoint != nil {
			list = append(list, name)
		}
	}
	sort.Strings(list)
	return list
}

// PrimaryAddr returns the server's primary endpoint as a TCPAddr.
func (ss *ServerSet) PrimaryAddr() *net.TCPAddr {
	return ss.ServiceEndpoint.Addr()
}

// NamedAddr returns the server's named endpoint as a TCPAddr.
func (ss *ServerSet) NamedAddr(namedPort string) *net.TCPAddr {
	if namedPort == "" {
		return ss.PrimaryAddr()
	}
	endpoint := ss.AdditionalEndpoints[namedPort]
	if endpoint == nil {
		panic(roxyutil.PortError{Type: roxyutil.NamedPort, Port: namedPort, Err: roxyutil.ErrNotExist})
	}
	return endpoint.Addr()
}

var _ Interface = (*ServerSet)(nil)

// }}}

// type ServerSetEndpoint {{{

// ServerSetEndpoint represents a single endpoint in a ServerSet advertisement.
//
// Each server's ServerSet can contain one primary and multiple named endpoints.
type ServerSetEndpoint struct {
	Host string `json:"host"`
	Port uint16 `json:"port"`
}

// Addr returns this endpoint as a TCPAddr.  Panics if the endpoint data cannot
// be parsed as an IP and port.
func (ep *ServerSetEndpoint) Addr() *net.TCPAddr {
	tcpAddr, err := TCPAddrFromServerSetEndpoint(ep)
	if err != nil {
		panic(err)
	}
	return tcpAddr
}

// }}}

// type ServerSetStatus {{{

// ServerSetStatus represents the status of the server being advertized.
type ServerSetStatus uint8

// ServerSetStatus constants:
//
//	StatusAlive    healthy
//	anything else  not healthy
const (
	StatusDead ServerSetStatus = iota
	StatusStarting
	StatusAlive
	StatusStopping
	StatusStopped
	StatusWarning
)

var serverSetStatusData = []roxyutil.EnumData{
	{
		GoName: "StatusDead",
		Name:   "DEAD",
		JSON:   []byte(`"DEAD"`),
	},
	{
		GoName: "StatusStarting",
		Name:   "STARTING",
		JSON:   []byte(`"STARTING"`),
	},
	{
		GoName: "StatusAlive",
		Name:   "ALIVE",
		JSON:   []byte(`"ALIVE"`),
	},
	{
		GoName: "StatusStopping",
		Name:   "STOPPING",
		JSON:   []byte(`"STOPPING"`),
	},
	{
		GoName: "StatusStopped",
		Name:   "STOPPED",
		JSON:   []byte(`"STOPPED"`),
	},
	{
		GoName: "StatusWarning",
		Name:   "WARNING",
		JSON:   []byte(`"WARNING"`),
	},
}

// GoString fulfills fmt.GoStringer.
func (status ServerSetStatus) GoString() string {
	return roxyutil.DereferenceEnumData("ServerSetStatus", serverSetStatusData, uint(status)).GoName
}

// String fulfills fmt.Stringer.
func (status ServerSetStatus) String() string {
	return roxyutil.DereferenceEnumData("ServerSetStatus", serverSetStatusData, uint(status)).Name
}

// MarshalJSON fulfills json.Marshaler.
func (status ServerSetStatus) MarshalJSON() ([]byte, error) {
	return roxyutil.MarshalEnumToJSON("ServerSetStatus", serverSetStatusData, uint(status))
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (status *ServerSetStatus) UnmarshalJSON(raw []byte) error {
	value, err := roxyutil.UnmarshalEnumFromJSON("ServerSetStatus", serverSetStatusData, raw)
	if err == nil {
		*status = ServerSetStatus(value)
		return nil
	}
	if err == roxyutil.ErrIsNull {
		return nil
	}
	*status = 0
	return err

}

var _ fmt.Stringer = ServerSetStatus(0)
var _ fmt.GoStringer = ServerSetStatus(0)
var _ json.Marshaler = ServerSetStatus(0)
var _ json.Unmarshaler = (*ServerSetStatus)(nil)

// }}}

// ServerSetEndpointFromTCPAddr is a helper function that creates a new
// ServerSetEndpoint representing the given TCPAddr.
func ServerSetEndpointFromTCPAddr(addr *net.TCPAddr) *ServerSetEndpoint {
	if addr == nil {
		return nil
	}

	ipAndZone := addr.IP.String()
	if addr.Zone != "" {
		ipAndZone += "%" + addr.Zone
	}

	return &ServerSetEndpoint{
		Host: ipAndZone,
		Port: uint16(addr.Port),
	}
}

// TCPAddrFromServerSetEndpoint is a helper function that parses an existing
// ServerSetEndpoint to produce a TCPAddr.
func TCPAddrFromServerSetEndpoint(ep *ServerSetEndpoint) (*net.TCPAddr, error) {
	if ep == nil {
		return nil, nil
	}

	ip, zone, err := misc.ParseIPAndZone(ep.Host)
	if err != nil {
		return nil, err
	}

	tcpAddr := &net.TCPAddr{
		IP:   ip,
		Port: int(ep.Port),
		Zone: zone,
	}
	return tcpAddr, nil
}
