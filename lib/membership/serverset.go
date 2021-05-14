package membership

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/misc"
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
	if !ss.IsAlive() {
		return nil
	}
	return ss.ServiceEndpoint.Addr()
}

// NamedAddr returns the server's named endpoint as a TCPAddr.
func (ss *ServerSet) NamedAddr(namedPort string) *net.TCPAddr {
	if namedPort == "" {
		return ss.PrimaryAddr()
	}
	if !ss.IsAlive() {
		return nil
	}
	endpoint := ss.AdditionalEndpoints[namedPort]
	if endpoint == nil {
		panic(fmt.Errorf("unknown named port %q", namedPort))
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
//
//	StatusAlive    healthy
//	anything else  not healthy
type ServerSetStatus uint8

const (
	StatusDead ServerSetStatus = iota
	StatusStarting
	StatusAlive
	StatusStopping
	StatusStopped
	StatusWarning
)

var serversetStatusData = []enumData{
	{"StatusDead", "DEAD"},
	{"StatusStarting", "STARTING"},
	{"StatusAlive", "ALIVE"},
	{"StatusStopping", "STOPPING"},
	{"StatusStopped", "STOPPED"},
	{"StatusWarning", "WARNING"},
}

var serversetStatusJSON = []enumJSON{
	{uint(StatusDead), []byte(`"DEAD"`)},
	{uint(StatusStarting), []byte(`"STARTING"`)},
	{uint(StatusAlive), []byte(`"ALIVE"`)},
	{uint(StatusStopping), []byte(`"STOPPING"`)},
	{uint(StatusStopped), []byte(`"STOPPED"`)},
	{uint(StatusWarning), []byte(`"WARNING"`)},
	{uint(StatusDead), []byte(`"dead"`)},
	{uint(StatusStarting), []byte(`"starting"`)},
	{uint(StatusAlive), []byte(`"alive"`)},
	{uint(StatusStopping), []byte(`"stopping"`)},
	{uint(StatusStopped), []byte(`"stopped"`)},
	{uint(StatusWarning), []byte(`"warning"`)},
	{uint(StatusDead), []byte("0")},
	{uint(StatusStarting), []byte("1")},
	{uint(StatusAlive), []byte("2")},
	{uint(StatusStopping), []byte("3")},
	{uint(StatusStopped), []byte("4")},
	{uint(StatusWarning), []byte("5")},
}

// GoString fulfills fmt.GoStringer.
func (status ServerSetStatus) GoString() string {
	if uint(status) >= uint(len(serversetStatusData)) {
		panic(fmt.Errorf("invalid ServerSetStatus value %d", uint(status)))
	}
	return serversetStatusData[status].GoName
}

// String fulfills fmt.Stringer.
func (status ServerSetStatus) String() string {
	if uint(status) >= uint(len(serversetStatusData)) {
		panic(fmt.Errorf("invalid ServerSetStatus value %d", uint(status)))
	}
	return serversetStatusData[status].Name
}

// MarshalJSON fulfills json.Marshaler.
func (status ServerSetStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(status.String())
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (status *ServerSetStatus) UnmarshalJSON(raw []byte) error {
	if raw == nil {
		panic(errors.New("raw is nil"))
	}

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	*status = ^ServerSetStatus(0)

	for _, row := range serversetStatusJSON {
		if bytes.Equal(raw, row.bytes) {
			*status = ServerSetStatus(row.index)
			return nil
		}
	}

	var str string
	err0 := json.Unmarshal(raw, &str)
	if err0 == nil {
		for index, data := range serversetStatusData {
			if strings.EqualFold(str, data.Name) || strings.EqualFold(str, data.GoName) {
				*status = ServerSetStatus(index)
				return nil
			}
		}
		return fmt.Errorf("invalid ServerSetStatus value %q", str)
	}

	var num uint8
	err1 := json.Unmarshal(raw, &num)
	if err1 == nil {
		if uint(num) < uint(len(serversetStatusData)) {
			*status = ServerSetStatus(num)
			return nil
		}
		return fmt.Errorf("invalid ServerSetStatus value %d", num)
	}

	return err0

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
