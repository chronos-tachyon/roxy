package main

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net"
	"path/filepath"
	"regexp"

	"github.com/go-zookeeper/zk"
	"github.com/rs/zerolog/log"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/certnames"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

var reSchemeAndPath = regexp.MustCompile(`^([A-Za-z][0-9A-Za-z+-]*):(.*)$`)

type GlobalConfigFile struct {
	ZK           mainutil.ZKConfig     `json:"zk"`
	Etcd         mainutil.EtcdConfig   `json:"etcd"`
	ListenGRPC   mainutil.ListenConfig `json:"listen"`
	ListenAdmin  mainutil.ListenConfig `json:"listenAdmin"`
	ListenProm   mainutil.ListenConfig `json:"listenPrometheus"`
	PeersFile    string                `json:"peersFile"`
	ServicesFile string                `json:"servicesFile"`
	CostFile     string                `json:"costFile"`
	GRPCAddr     *net.TCPAddr          `json:"-"`
}

type PeersFile []string

type ServicesFile map[string]ServiceConfig

type ServiceConfig struct {
	AllowedClientNames         certnames.CertNames `json:"allowedClientNames"`
	AllowedServerNames         certnames.CertNames `json:"allowedServerNames"`
	ExpectedNumClientsPerShard uint32              `json:"expectedNumClientsPerShard"`
	ExpectedNumServersPerShard uint32              `json:"expectedNumServersPerShard"`
	IsSharded                  bool                `json:"isSharded"`
	NumShards                  uint32              `json:"numShards"`
	AvgSuppliedCPSPerServer    float64             `json:"avgSuppliedCPSPerServer"`
	AvgDemandedCPQ             float64             `json:"avgDemandedCPQ"`
}

type CostFile []CostConfig

type CostConfig struct {
	A    string  `json:"a"`
	B    string  `json:"b"`
	Cost float32 `json:"cost"`
}

func LoadGlobalConfigFile() (*GlobalConfigFile, error) {
	path, err := roxyutil.ExpandPath(flagConfig)
	if err != nil {
		return nil, err
	}
	flagConfig = path

	dir := filepath.Dir(flagConfig)

	file := &GlobalConfigFile{
		ListenAdmin: mainutil.ListenConfig{
			Enabled: true,
			Network: constants.NetUnix,
			Address: "/var/opt/roxy/lib/atc.admin.socket",
		},
		ListenProm: mainutil.ListenConfig{
			Enabled: true,
			Network: constants.NetTCP,
			Address: "127.0.0.1:6801",
		},
		ListenGRPC: mainutil.ListenConfig{
			Enabled: true,
			Network: constants.NetTCP,
			Address: "127.0.0.1:2987",
		},
		PeersFile:    "peers.json",
		ServicesFile: "services.json",
		CostFile:     "cost.json",
	}

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = misc.StrictUnmarshalJSON(raw, file)
	if err != nil {
		return nil, err
	}

	for _, lc := range []*mainutil.ListenConfig{
		&file.ListenAdmin,
		&file.ListenProm,
	} {
		if lc.Enabled && constants.IsNetUnix(lc.Network) {
			lc.Address, err = roxyutil.ExpandPath(lc.Address)
			if err != nil {
				return nil, err
			}
		}
	}

	if !file.ListenGRPC.Enabled {
		return nil, fmt.Errorf("must enable the primary port")
	}

	if !constants.IsNetTCP(file.ListenGRPC.Network) {
		return nil, fmt.Errorf("primary port must listen on TCP, not %q", file.ListenGRPC.Network)
	}

	addr, err := misc.ParseTCPAddr(file.ListenGRPC.Address, constants.PortATC)
	if err != nil {
		return nil, err
	}
	if addr.IP.IsUnspecified() {
		return nil, fmt.Errorf("primary port must listen on a specific IP address, not %s", addr.IP)
	}
	addr = misc.CanonicalizeTCPAddr(addr)
	file.GRPCAddr = addr
	file.ListenGRPC.Address = addr.String()

	for _, ptr := range []*string{
		&file.PeersFile,
		&file.ServicesFile,
		&file.CostFile,
	} {
		*ptr, err = processPathWithMaybeScheme(*ptr, dir)
		if err != nil {
			return nil, err
		}
	}

	log.Logger.Trace().
		Str("subsystem", constants.SubsystemGRPC).
		Interface("config", file.ListenGRPC).
		Msg("ready")
	log.Logger.Trace().
		Str("subsystem", constants.SubsystemAdmin).
		Interface("config", file.ListenAdmin).
		Msg("ready")
	log.Logger.Trace().
		Str("subsystem", constants.SubsystemProm).
		Interface("config", file.ListenProm).
		Msg("ready")

	return file, nil
}

func (file *GlobalConfigFile) LoadPeersFile(ctx context.Context, zkConn *zk.Conn, etcd *v3.Client, rev int64) (PeersFile, error) {
	var out PeersFile
	err := loadConfigFileWithScheme(ctx, zkConn, etcd, file.PeersFile, rev, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (file *GlobalConfigFile) LoadServicesFile(ctx context.Context, zkConn *zk.Conn, etcd *v3.Client, rev int64) (ServicesFile, error) {
	var out ServicesFile
	err := loadConfigFileWithScheme(ctx, zkConn, etcd, file.ServicesFile, rev, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (file *GlobalConfigFile) LoadCostFile(ctx context.Context, zkConn *zk.Conn, etcd *v3.Client, rev int64) (CostFile, error) {
	var out CostFile
	err := loadConfigFileWithScheme(ctx, zkConn, etcd, file.CostFile, rev, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func processPathWithMaybeScheme(input string, dir string) (string, error) {
	var scheme, path string
	match := reSchemeAndPath.FindStringSubmatch(input)
	if match == nil {
		scheme = constants.SchemeFile
		path = input
	} else {
		scheme = match[1]
		path = match[2]
	}

	var err error
	switch scheme {
	case constants.SchemeFile:
		path, err = roxyutil.ExpandPathWithCWD(path, dir)

	case constants.SchemeZK:
		path, err = roxyutil.ExpandString(path)

	case constants.SchemeEtcd:
		path, err = roxyutil.ExpandString(path)

	default:
		return "", fmt.Errorf("unknown scheme %q", scheme)
	}
	if err != nil {
		return "", err
	}

	return scheme + ":" + path, nil
}

func loadConfigFileWithScheme(ctx context.Context, zkConn *zk.Conn, etcd *v3.Client, schemeAndPath string, rev int64, v interface{}) error {
	var scheme, path string
	match := reSchemeAndPath.FindStringSubmatch(schemeAndPath)
	if match == nil {
		scheme = constants.SchemeFile
		path = schemeAndPath
	} else {
		scheme = match[1]
		path = match[2]
	}

	log.Logger.Trace().
		Str("scheme", scheme).
		Str("path", path).
		Int64("rev", rev).
		Msg("loadConfigFileWithScheme")

	var raw []byte
	var err error
	switch scheme {
	case constants.SchemeFile:
		raw, err = ioutil.ReadFile(path)
		if err != nil {
			return err
		}

	case constants.SchemeZK:
		raw, _, err = zkConn.Get(path)
		if err != nil {
			return err
		}

	case constants.SchemeEtcd:
		opts := make([]v3.OpOption, 0, 1)
		if rev != 0 {
			opts = append(opts, v3.WithRev(rev))
		}

		var resp *v3.GetResponse
		resp, err = etcd.KV.Get(ctx, path, opts...)
		if err != nil {
			return err
		}

		if len(resp.Kvs) == 0 {
			return fs.ErrNotExist
		}

		raw = resp.Kvs[0].Value

	default:
		return fmt.Errorf("scheme %q not implemented", scheme)
	}

	return misc.StrictUnmarshalJSON(raw, v)
}
