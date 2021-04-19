package enums

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ResolverType uint8

const (
	DefaultResolver ResolverType = iota
	UnixResolver
	DNSResolver
	SRVResolver
	EtcdResolver
	ZookeeperResolver
)

var resolverTypeData = []enumData{
	{"DefaultResolver", ""},
	{"UnixResolver", "unix"},
	{"DNSResolver", "dns"},
	{"SRVResolver", "srv"},
	{"EtcdResolver", "etcd"},
	{"ZookeeperResolver", "zk"},
}

var resolverTypeMap = map[string]ResolverType{
	"":          DefaultResolver,
	"unix":      UnixResolver,
	"dns":       DNSResolver,
	"srv":       SRVResolver,
	"etcd":      EtcdResolver,
	"zk":        ZookeeperResolver,
	"zookeeper": ZookeeperResolver,
}

func (t ResolverType) String() string {
	if uint(t) >= uint(len(resolverTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return resolverTypeData[t].Name
}

func (t ResolverType) GoString() string {
	if uint(t) >= uint(len(resolverTypeData)) {
		return fmt.Sprintf("ResolverType(%d)", uint(t))
	}
	return resolverTypeData[t].GoName
}

func (t ResolverType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (ptr *ResolverType) UnmarshalJSON(raw []byte) error {
	*ptr = 0

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	if num, ok := resolverTypeMap[strings.ToLower(str)]; ok {
		*ptr = num
		return nil
	}

	return fmt.Errorf("illegal resolver type %q; expected one of %q", str, makeAllowedNames(resolverTypeData))
}

var _ fmt.Stringer = ResolverType(0)
var _ fmt.GoStringer = ResolverType(0)
var _ json.Marshaler = ResolverType(0)
var _ json.Unmarshaler = (*ResolverType)(nil)
