package membership

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"

	"github.com/chronos-tachyon/roxy/internal/misc"
)

type Roxy struct {
	Ready           bool
	IP              net.IP
	Zone            string
	ServerName      string
	PrimaryPort     uint16
	AdditionalPorts map[string]uint16
	ShardID         *uint32
	Metadata        map[string]string
}

type RoxyJSON struct {
	Ready           bool              `json:"ready"`
	IP              string            `json:"ip,omitempty"`
	Zone            string            `json:"zone,omitempty"`
	ServerName      string            `json:"serverName,omitempty"`
	PrimaryPort     uint16            `json:"primaryPort,omitempty"`
	AdditionalPorts map[string]uint16 `json:"additionalPorts,omitempty"`
	ShardID         *uint32           `json:"shardID,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

func (r *Roxy) NamedPorts() []string {
	if !r.Ready {
		return nil
	}
	list := make([]string, 0, len(r.AdditionalPorts))
	for name := range r.AdditionalPorts {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

func (r *Roxy) PrimaryAddr() *net.TCPAddr {
	if !r.Ready {
		return nil
	}
	return &net.TCPAddr{
		IP:   r.IP,
		Zone: r.Zone,
		Port: int(r.PrimaryPort),
	}
}

func (r *Roxy) NamedAddr(namedPort string) *net.TCPAddr {
	if namedPort == "" {
		return r.PrimaryAddr()
	}
	if !r.Ready {
		return nil
	}
	port, ok := r.AdditionalPorts[namedPort]
	if !ok {
		return nil
	}
	return &net.TCPAddr{
		IP:   r.IP,
		Zone: r.Zone,
		Port: int(port),
	}
}

func (r *Roxy) AsRoxyJSON() (*RoxyJSON, error) {
	out := &RoxyJSON{
		Ready:           r.Ready,
		IP:              r.IP.String(),
		Zone:            r.Zone,
		PrimaryPort:     r.PrimaryPort,
		AdditionalPorts: r.AdditionalPorts,
		ServerName:      r.ServerName,
		ShardID:         r.ShardID,
		Metadata:        r.Metadata,
	}
	return out, nil
}

func (r *Roxy) AsServerSet() (*ServerSet, error) {
	status := StatusDead
	if r.Ready {
		status = StatusAlive
	}

	primary := ServerSetEndpointFromTCPAddr(r.PrimaryAddr())

	additional := make(map[string]*ServerSetEndpoint, len(r.AdditionalPorts))
	for name := range r.AdditionalPorts {
		additional[name] = ServerSetEndpointFromTCPAddr(r.NamedAddr(name))
	}

	var shardID *int32
	if r.ShardID != nil {
		if *r.ShardID >= uint32(1<<31) {
			return nil, fmt.Errorf("ShardID %d is out of range", *r.ShardID)
		}
		shardID = new(int32)
		*shardID = int32(*r.ShardID)
	}

	metadata := make(map[string]string, 1)
	if r.ServerName != "" {
		metadata["ServerName"] = r.ServerName
	}

	out := &ServerSet{
		ServiceEndpoint:     primary,
		AdditionalEndpoints: additional,
		Status:              status,
		ShardID:             shardID,
		Metadata:            metadata,
	}
	return out, nil
}

func (r *Roxy) AsGRPC(namedPort string) (*GRPC, error) {
	op := GRPCOpDelete
	if r.Ready {
		op = GRPCOpAdd
	}

	tcpAddr := r.NamedAddr(namedPort)
	if tcpAddr == nil {
		return nil, fmt.Errorf("unknown named port %q", namedPort)
	}

	metadata := make(map[string]interface{}, 8)
	if r.ServerName != "" {
		metadata["ServerName"] = r.ServerName
	}
	if r.ShardID != nil {
		metadata["ShardID"] = *r.ShardID
	}

	out := &GRPC{
		Op:       op,
		Addr:     tcpAddr.String(),
		Metadata: metadata,
	}
	return out, nil
}

func (r *Roxy) FromRoxyJSON(x *RoxyJSON) error {
	if x == nil {
		panic(errors.New("*membership.RoxyJSON is nil"))
	}

	ip, err := misc.ParseIP(x.IP)
	if err != nil {
		return err
	}

	*r = Roxy{
		Ready:           x.Ready,
		IP:              ip,
		Zone:            x.Zone,
		ServerName:      x.ServerName,
		PrimaryPort:     x.PrimaryPort,
		AdditionalPorts: x.AdditionalPorts,
		ShardID:         x.ShardID,
		Metadata:        x.Metadata,
	}
	return nil
}

func (r *Roxy) FromServerSet(ss *ServerSet) error {
	if ss == nil {
		panic(errors.New("*membership.ServerSet is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*r = Roxy{}
		}
	}()

	if ss.Status != StatusAlive {
		return nil
	}

	r.Ready = true

	tcpAddr, err := TCPAddrFromServerSetEndpoint(ss.ServiceEndpoint)
	if err != nil {
		return err
	}

	r.IP = tcpAddr.IP
	r.Zone = tcpAddr.Zone
	r.PrimaryPort = uint16(tcpAddr.Port)

	r.AdditionalPorts = make(map[string]uint16, len(ss.AdditionalEndpoints))
	for namedPort, ep := range ss.AdditionalEndpoints {
		tcpAddr2, err := TCPAddrFromServerSetEndpoint(ep)
		if err != nil {
			return err
		}
		if !tcpAddr.IP.Equal(tcpAddr2.IP) {
			return fmt.Errorf("ServerSet with multiple IP addresses not supported: %s vs %s", tcpAddr.IP, tcpAddr2.IP)
		}
		if tcpAddr.Zone != tcpAddr2.Zone {
			return fmt.Errorf("ServerSet with multiple IPv6 zones not supported: %q vs %q", tcpAddr.Zone, tcpAddr2.Zone)
		}
		r.AdditionalPorts[namedPort] = uint16(tcpAddr2.Port)
	}

	if ss.ShardID != nil {
		if *ss.ShardID < 0 {
			return fmt.Errorf("ServerSet with negative ShardID not supported: %d", *ss.ShardID)
		}
		r.ShardID = new(uint32)
		*r.ShardID = uint32(*ss.ShardID)
	}

	r.Metadata = make(map[string]string, len(ss.Metadata))
	for key, value := range ss.Metadata {
		r.Metadata[key] = value
	}

	if str, found := r.Metadata["ServerName"]; found {
		delete(r.Metadata, "ServerName")
		r.ServerName = str
	}

	wantZero = false
	return nil
}

func (r *Roxy) FromGRPC(grpc *GRPC) error {
	if grpc == nil {
		panic(errors.New("*membership.GRPC is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*r = Roxy{}
		}
	}()

	if grpc.Op != GRPCOpAdd {
		return nil
	}

	r.Ready = true

	tcpAddr, err := misc.ParseTCPAddr(grpc.Addr, "")
	if err != nil {
		return err
	}

	r.IP = tcpAddr.IP
	r.Zone = tcpAddr.Zone
	r.PrimaryPort = uint16(tcpAddr.Port)

	switch x := grpc.Metadata.(type) {
	case map[string]interface{}:
		r.Metadata = make(map[string]string, len(x))
		for key, y := range x {
			r.Metadata[key] = fmt.Sprint(y)
		}

	case map[string]string:
		r.Metadata = make(map[string]string, len(x))
		for key, value := range x {
			r.Metadata[key] = value
		}
	}

	if str, found := r.Metadata["ServerName"]; found {
		delete(r.Metadata, "ServerName")
		r.ServerName = str
	}

	if str, found := r.Metadata["ShardID"]; found {
		if u64, err := strconv.ParseUint(str, 10, 32); err == nil {
			delete(r.Metadata, "ShardID")
			shardID := new(uint32)
			*shardID = uint32(u64)
			r.ShardID = shardID
		}
	}

	wantZero = false
	return nil
}

func (r *Roxy) MarshalJSON() ([]byte, error) {
	if r == nil {
		return nullBytes, nil
	}

	x, err := r.AsRoxyJSON()
	if err != nil {
		return nil, err
	}

	return json.Marshal(x)
}

func (r *Roxy) UnmarshalJSON(raw []byte) error {
	if raw == nil {
		panic(errors.New("raw is nil"))
	}

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var x RoxyJSON
	err := misc.StrictUnmarshalJSON(raw, &x)
	if err != nil {
		return err
	}

	return r.FromRoxyJSON(&x)
}

func (r *Roxy) Parse(raw []byte) error {
	if r == nil {
		panic(errors.New("*Roxy is nil"))
	}

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	wantZero := true
	defer func() {
		if wantZero {
			*r = Roxy{}
		}
	}()

	err0 := misc.StrictUnmarshalJSON(raw, r)
	if err0 == nil {
		wantZero = false
		return nil
	}

	var ss ServerSet
	err1 := misc.StrictUnmarshalJSON(raw, &ss)
	if err1 == nil {
		err := r.FromServerSet(&ss)
		if err != nil {
			return err
		}

		wantZero = false
		return nil
	}

	var grpc GRPC
	err2 := misc.StrictUnmarshalJSON(raw, &grpc)
	if err2 == nil {
		err := r.FromGRPC(&grpc)
		if err != nil {
			return err
		}

		wantZero = false
		return nil
	}

	return fmt.Errorf("failed to parse JSON: `%s`: %w", string(raw), err0)
}
