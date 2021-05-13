package membership

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/misc"
)

type ServerSet struct {
	ServiceEndpoint     *ServerSetEndpoint            `json:"serviceEndpoint"`
	AdditionalEndpoints map[string]*ServerSetEndpoint `json:"additionalEndpoints"`
	Status              ServerSetStatus               `json:"status"`
	ShardID             *int32                        `json:"shardId,omitempty"`
	Metadata            map[string]string             `json:"metadata,omitempty"`
}

func (ss *ServerSet) AsGRPC(namedPort string) *GRPC {
	out := new(GRPC)
	if ss.Status != StatusAlive {
		out.Op = GRPCOpDelete
		return out
	}

	tcpAddr := ss.TCPAddrForPort(namedPort)
	if tcpAddr == nil {
		panic(fmt.Errorf("unknown named port %q", namedPort))
	}

	metadata := make(map[string]interface{}, 2+len(ss.Metadata))
	for key, value := range ss.Metadata {
		metadata[key] = value
	}
	if ss.ShardID != nil {
		metadata["ShardID"] = *ss.ShardID
	}

	out.Op = GRPCOpAdd
	out.Addr = tcpAddr.String()
	out.Metadata = metadata
	return out
}

func (ss *ServerSet) AsJSON() []byte {
	raw, err := json.Marshal(ss)
	if err != nil {
		panic(fmt.Errorf("failed to json.Marshal your ServerSet: %w", err))
	}
	return raw
}

func (ss *ServerSet) IsAlive() bool {
	if ss == nil {
		return false
	}
	return ss.Status == StatusAlive
}

func (ss *ServerSet) TCPAddrForPort(namedPort string) *net.TCPAddr {
	if ss == nil {
		return nil
	}
	ep := ss.ServiceEndpoint
	if namedPort != "" {
		ep = ss.AdditionalEndpoints[namedPort]
	}
	return ep.TCPAddr()
}

type ServerSetEndpoint struct {
	Host string `json:"host"`
	Port uint16 `json:"port"`
}

func (ep *ServerSetEndpoint) TCPAddr() *net.TCPAddr {
	if ep == nil {
		return nil
	}
	tcpAddr, err := TCPAddrFromServerSetEndpoint(ep)
	if err != nil {
		panic(err)
	}
	return tcpAddr
}

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

var serversetStatusMap = map[string]ServerSetStatus{
	"0":        StatusDead,
	"1":        StatusStarting,
	"2":        StatusAlive,
	"3":        StatusStopping,
	"4":        StatusStopped,
	"5":        StatusWarning,
	"DEAD":     StatusDead,
	"STARTING": StatusStarting,
	"ALIVE":    StatusAlive,
	"STOPPING": StatusStopping,
	"STOPPED":  StatusStopped,
	"WARNING":  StatusWarning,
}

func (st ServerSetStatus) String() string {
	if uint(st) >= uint(len(serversetStatusData)) {
		return fmt.Sprintf("#%d", uint(st))
	}
	return serversetStatusData[st].Name
}

func (st ServerSetStatus) GoString() string {
	if uint(st) >= uint(len(serversetStatusData)) {
		return fmt.Sprintf("ServerSetStatus(%d)", uint(st))
	}
	return serversetStatusData[st].GoName
}

func (st ServerSetStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(st.String())
}

func (st *ServerSetStatus) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	if num, found := serversetStatusMap[string(raw)]; found {
		*st = num
		return nil
	}

	var str string
	err0 := json.Unmarshal(raw, &str)
	if err0 == nil {
		if num, found := serversetStatusMap[str]; found {
			*st = num
			return nil
		}
		for index, data := range serversetStatusData {
			if strings.EqualFold(str, data.Name) {
				*st = ServerSetStatus(index)
				return nil
			}
		}
		*st = 0
		return fmt.Errorf("unknown ServerSetStatus %q", str)
	}

	var num uint8
	err1 := json.Unmarshal(raw, &num)
	if err1 == nil {
		*st = ServerSetStatus(num)
		return nil
	}

	*st = 0
	return err0

}

var _ fmt.Stringer = ServerSetStatus(0)
var _ fmt.GoStringer = ServerSetStatus(0)
var _ json.Marshaler = ServerSetStatus(0)
var _ json.Unmarshaler = (*ServerSetStatus)(nil)

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

func TCPAddrFromServerSetEndpoint(ep *ServerSetEndpoint) (*net.TCPAddr, error) {
	if ep == nil {
		panic(errors.New("*ServerSetEndpoint is nil"))
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

type enumData struct {
	GoName string
	Name   string
}