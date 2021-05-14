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

type Interface interface {
	IsAlive() bool
	NamedPorts() []string
	PrimaryAddr() *net.TCPAddr
	NamedAddr(string) *net.TCPAddr
}

// type Roxy {{{

type Roxy struct {
	Ready           bool
	IP              net.IP
	Zone            string
	ServerName      string
	PrimaryPort     uint16
	AdditionalPorts map[string]uint16
	ShardID         uint32
	HasShardID      bool
	Metadata        map[string]string
}

// IsAlive returns true if this represents the advertisement of a live server.
func (r *Roxy) IsAlive() bool {
	return r != nil && r.Ready
}

// NamedPorts returns the list of named ports advertized by the server.
func (r *Roxy) NamedPorts() []string {
	if !r.IsAlive() {
		return nil
	}
	list := make([]string, 0, len(r.AdditionalPorts))
	for name := range r.AdditionalPorts {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

// PrimaryAddr returns the server's primary endpoint as a TCPAddr.
func (r *Roxy) PrimaryAddr() *net.TCPAddr {
	if !r.IsAlive() {
		return nil
	}
	return &net.TCPAddr{
		IP:   r.IP,
		Port: int(r.PrimaryPort),
		Zone: r.Zone,
	}
}

// NamedAddr returns the server's named endpoint as a TCPAddr.
func (r *Roxy) NamedAddr(namedPort string) *net.TCPAddr {
	if namedPort == "" {
		return r.PrimaryAddr()
	}
	if !r.IsAlive() {
		return nil
	}
	port, ok := r.AdditionalPorts[namedPort]
	if !ok {
		panic(fmt.Errorf("unknown named port %q", namedPort))
	}
	return &net.TCPAddr{
		IP:   r.IP,
		Port: int(port),
		Zone: r.Zone,
	}
}

// MarshalJSON fulfills json.Marshaler.
func (r *Roxy) MarshalJSON() ([]byte, error) {
	if r == nil {
		return nullBytes, nil
	}
	return json.Marshal(r.AsRoxyJSON())
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (r *Roxy) UnmarshalJSON(raw []byte) error {
	if raw == nil {
		panic(errors.New("raw is nil"))
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

	var x RoxyJSON
	err0 := misc.StrictUnmarshalJSON(raw, &x)
	if err0 == nil {
		err := r.FromRoxyJSON(&x)
		if err == nil {
			wantZero = false
			return nil
		}
		return err
	}

	var y ServerSet
	err1 := misc.StrictUnmarshalJSON(raw, &y)
	if err1 == nil {
		err := r.FromServerSet(&y)
		if err == nil {
			wantZero = false
			return nil
		}
		return err
	}

	var z GRPC
	err2 := misc.StrictUnmarshalJSON(raw, &z)
	if err2 == nil {
		err := r.FromGRPC(&z)
		if err == nil {
			wantZero = false
			return nil
		}
		return err
	}

	return err0
}

func (r *Roxy) AsRoxyJSON() *RoxyJSON {
	if r == nil {
		return nil
	}

	ip := r.IP.String()

	var shardID *uint32
	if r.HasShardID {
		shardID = new(uint32)
		*shardID = r.ShardID
	}

	out := &RoxyJSON{
		Ready:           r.Ready,
		IP:              ip,
		Zone:            r.Zone,
		PrimaryPort:     r.PrimaryPort,
		AdditionalPorts: r.AdditionalPorts,
		ServerName:      r.ServerName,
		ShardID:         shardID,
		Metadata:        r.Metadata,
	}
	return out
}

func (r *Roxy) AsServerSet() *ServerSet {
	if r == nil {
		return nil
	}

	status := StatusDead
	if r.IsAlive() {
		status = StatusAlive
	}

	primary := ServerSetEndpointFromTCPAddr(r.PrimaryAddr())

	additional := make(map[string]*ServerSetEndpoint, len(r.AdditionalPorts))
	for name := range r.AdditionalPorts {
		additional[name] = ServerSetEndpointFromTCPAddr(r.NamedAddr(name))
	}

	var shardID *int32
	if r.HasShardID {
		shardID = new(int32)
		*shardID = int32(r.ShardID)
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
	return out
}

func (r *Roxy) AsGRPC(namedPort string) *GRPC {
	if r == nil {
		return nil
	}

	op := GRPCOpDelete
	if r.IsAlive() {
		op = GRPCOpAdd
	}

	tcpAddr := r.NamedAddr(namedPort)

	metadata := make(map[string]interface{}, 8)
	if r.ServerName != "" {
		metadata["ServerName"] = r.ServerName
	}
	if r.HasShardID {
		metadata["ShardID"] = r.ShardID
	}

	out := &GRPC{
		Op:       op,
		Addr:     tcpAddr.String(),
		Metadata: metadata,
	}
	return out
}

func (r *Roxy) FromRoxyJSON(x *RoxyJSON) error {
	if r == nil {
		panic(errors.New("*membership.Roxy is nil"))
	}
	if x == nil {
		panic(errors.New("*membership.RoxyJSON is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*r = Roxy{}
		}
	}()

	if !x.Ready {
		return nil
	}

	r.Ready = true
	r.Zone = x.Zone
	r.ServerName = x.ServerName
	r.PrimaryPort = x.PrimaryPort
	r.AdditionalPorts = x.AdditionalPorts
	r.Metadata = x.Metadata

	var err error
	r.IP, err = misc.ParseIP(x.IP)
	if err != nil {
		return err
	}

	if x.ShardID != nil {
		r.ShardID = *x.ShardID
		r.HasShardID = true
	}

	wantZero = false
	return nil
}

func (r *Roxy) FromServerSet(ss *ServerSet) error {
	if r == nil {
		panic(errors.New("*membership.Roxy is nil"))
	}
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
		r.ShardID = uint32(*ss.ShardID)
		r.HasShardID = true
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
	if r == nil {
		panic(errors.New("*membership.Roxy is nil"))
	}
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
			r.ShardID = uint32(u64)
			r.HasShardID = true
		}
	}

	wantZero = false
	return nil
}

var _ Interface = (*Roxy)(nil)

// }}}

// type RoxyJSON {{{

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

// }}}
