package membership

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

type ServerSet struct {
	ServiceEndpoint     *ServerSetEndpoint            `json:"serviceEndpoint"`
	AdditionalEndpoints map[string]*ServerSetEndpoint `json:"additionalEndpoints"`
	Status              ServerSetStatus               `json:"status"`
	ShardID             *int32                        `json:"shardId,omitempty"`
	Metadata            map[string]string             `json:"metadata,omitempty"`
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

func (ss *ServerSet) TCPAddrForPort(portName string) *net.TCPAddr {
	if ss == nil {
		return nil
	}
	if portName == "" {
		return ss.ServiceEndpoint.TCPAddr()
	}
	return ss.AdditionalEndpoints[portName].TCPAddr()
}

type ServerSetEndpoint struct {
	Host string `json:"host"`
	Port uint16 `json:"port"`
	IP   net.IP `json:"-"`
	Zone string `json:"-"`
}

func (ep *ServerSetEndpoint) TCPAddr() *net.TCPAddr {
	if ep == nil {
		return nil
	}
	return &net.TCPAddr{
		IP:   ep.IP,
		Port: int(ep.Port),
		Zone: ep.Zone,
	}
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

type enumData struct {
	GoName string
	Name   string
}
