// Command "atcctl" is the admin CLI for "atc".
//
// Usage:
//
//	atcctl [<flags>] <cmd> [<args>...]
//
// Flags:
//
//	-V, --version        print version and exit
//	-s, --server=addr    Address of the ATC admin port
//	  [default: "unix:/var/opt/roxy/lib/atc.admin.socket"]
//	-J, --log-journald   log to journald
//	-l, --log-file=path  log JSON to file
//	-S, --log-stderr     log JSON to stderr
//	-v, --verbose        enable debug logging
//	-d, --debug          enable debug and trace logging
//
// Commands:
//
//	help                                     list available commands
//	ping                                     check that ATC is running
//	reload id [rev]                          tell ATC to rotate its log file and to pre-load its config
//	flip id                                  tell ATC to activate a preloaded config
//	commit id                                tell ATC to commit to an activated config
//	shutdown                                 tell ATC to exit
//	healthcheck [name]                       check the health of the named subsystem(*)
//	healthwatch [name]                       watch the health of the named subsystem(*)
//	set-health name y/n                      set the health of the named subsystem
//	lookup service [shard]                   perform a /roxy.v0.AirTrafficControl/Lookup RPC
//	lookup-clients service [shard] [unique]  perform a /roxy.v0.AirTrafficControl/LookupClients RPC
//	lookup-servers service [shard] [unique]  perform a /roxy.v0.AirTrafficControl/LookupServers RPC
//	find service [shard]                     perform a /roxy.v0.AirTrafficControl/Find RPC
//
//	(*) At startup, the only available subsystem is the empty string, "".
//          The "set-health" command can create new named subsystems.
//
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

const helpText = `atcctl [<flags>] <cmd> [<args>...]
Commands available:
	help
	ping
	reload <id> [<rev>]
	flip <id>
	commit <id>
	shutdown
	healthcheck [<subsystem>]
	healthwatch [<subsystem>]
	set-health <subsystem> <value>
	lookup <service> [<shard-id>]
	lookup-clients <service> <shard-id> [<unique>]
	lookup-servers <service> <shard-id> [<unique>]
	find <service> [<shard-id>]

`

var (
	flagAdminTarget string = "unix:/var/opt/roxy/lib/atc.admin.socket"
)

const (
	cmdPing          = "ping"
	cmdReload        = "reload"
	cmdFlip          = "flip"
	cmdCommit        = "commit"
	cmdShutdown      = "shutdown"
	cmdHealthCheck   = "healthcheck"
	cmdHealthWatch   = "healthwatch"
	cmdSetHealth     = "set-health"
	cmdLookup        = "lookup"
	cmdLookupClients = "lookup-clients"
	cmdLookupServers = "lookup-servers"
	cmdFind          = "find"
)

type clients struct {
	cc     *grpc.ClientConn
	health grpc_health_v1.HealthClient
	admin  roxy_v0.AdminClient
	atc    roxy_v0.AirTrafficControlClient
}

type commandFunc func(context.Context, clients, *zerolog.Event, []string) *zerolog.Event

type commandRecord struct {
	fn       commandFunc
	minNArgs uint
	maxNArgs uint
}

var aliasMap = map[string]string{
	"health":        cmdHealthCheck,
	"health-check":  cmdHealthCheck,
	"check-health":  cmdHealthCheck,
	"health-watch":  cmdHealthWatch,
	"watch-health":  cmdHealthWatch,
	"lookup-client": cmdLookupClients,
	"lookup-server": cmdLookupServers,
}

var commandMap = map[string]commandRecord{
	cmdPing: {
		fn:       doPing,
		minNArgs: 1,
		maxNArgs: 1,
	},
	cmdReload: {
		fn:       doReload,
		minNArgs: 2,
		maxNArgs: 3,
	},
	cmdFlip: {
		fn:       doFlip,
		minNArgs: 2,
		maxNArgs: 2,
	},
	cmdCommit: {
		fn:       doCommit,
		minNArgs: 2,
		maxNArgs: 2,
	},
	cmdShutdown: {
		fn:       doShutdown,
		minNArgs: 1,
		maxNArgs: 1,
	},
	cmdHealthCheck: {
		fn:       doHealthCheck,
		minNArgs: 1,
		maxNArgs: 2,
	},
	cmdHealthWatch: {
		fn:       doHealthWatch,
		minNArgs: 1,
		maxNArgs: 2,
	},
	cmdSetHealth: {
		fn:       doSetHealth,
		minNArgs: 3,
		maxNArgs: 3,
	},
	cmdLookup: {
		fn:       doLookup,
		minNArgs: 2,
		maxNArgs: 3,
	},
	cmdLookupClients: {
		fn:       doLookupClients,
		minNArgs: 2,
		maxNArgs: 4,
	},
	cmdLookupServers: {
		fn:       doLookupServers,
		minNArgs: 2,
		maxNArgs: 4,
	},
	cmdFind: {
		fn:       doFind,
		minNArgs: 2,
		maxNArgs: 3,
	},
}

func init() {
	getopt.SetParameters("<cmd> [<arg>...]")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagAdminTarget, "server", 's', "address for Admin gRPC interface")
}

func main() {
	getopt.Parse()

	mainutil.InitVersion()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	fn, _, args := processCommandAndArgs()

	var adminConfig mainutil.GRPCClientConfig
	err := adminConfig.Parse(flagAdminTarget)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagAdminTarget).
			Err(err).
			Msg("--server: failed to parse")
	}

	cc, err := adminConfig.Dial(ctx)
	if err != nil {
		log.Logger.Fatal().
			Interface("config", adminConfig).
			Err(err).
			Msg("--server: failed to Dial")
	}
	defer func() {
		_ = cc.Close()
	}()

	c := clients{
		cc:     cc,
		health: grpc_health_v1.NewHealthClient(cc),
		admin:  roxy_v0.NewAdminClient(cc),
		atc:    roxy_v0.NewAirTrafficControlClient(cc),
	}

	exitCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Ignore(syscall.SIGHUP)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		select {
		case <-sigCh:
			signal.Stop(sigCh)
			mainutil.CancelRootContext()
		case <-exitCh:
			// pass
		}
		wg.Done()
	}()

	event := log.Logger.Info()
	event = fn(ctx, c, event, args)
	event.Msg("OK")

	close(exitCh)
	wg.Wait()
}

func doPing(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	_, err := c.admin.Ping(ctx, &roxy_v0.PingRequest{})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.Admin").
			Str("rpcMethod", "Ping").
			Err(err).
			Msg("RPC failed")
	}
	return event
}

func doReload(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	configStr := args[0]
	revStr := args[1]

	configID, err := strconv.ParseUint(configStr, 10, 64)
	if err != nil {
		log.Logger.Fatal().
			Str("input", configStr).
			Err(err).
			Msg("invalid ConfigId")
	}

	var rev int64
	if revStr != "" {
		rev, err = strconv.ParseInt(revStr, 10, 64)
		if err != nil {
			log.Logger.Fatal().
				Str("input", revStr).
				Err(err).
				Msg("invalid revision")
		}
	}

	_, err = c.admin.Reload(ctx, &roxy_v0.ReloadRequest{Id: configID, Rev: rev})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.Admin").
			Str("rpcMethod", "Reload").
			Err(err).
			Msg("RPC failed")
	}

	return event
}

func doFlip(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	configStr := args[0]

	configID, err := strconv.ParseUint(configStr, 10, 64)
	if err != nil {
		log.Logger.Fatal().
			Str("input", configStr).
			Err(err).
			Msg("invalid ConfigId")
	}

	_, err = c.admin.Flip(ctx, &roxy_v0.FlipRequest{Id: configID})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.Admin").
			Str("rpcMethod", "Flip").
			Err(err).
			Msg("RPC failed")
	}

	return event
}

func doCommit(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	configStr := args[0]

	configID, err := strconv.ParseUint(configStr, 10, 64)
	if err != nil {
		log.Logger.Fatal().
			Str("input", configStr).
			Err(err).
			Msg("invalid ConfigId")
	}

	_, err = c.admin.Commit(ctx, &roxy_v0.CommitRequest{Id: configID})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.Admin").
			Str("rpcMethod", "Commit").
			Err(err).
			Msg("RPC failed")
	}

	return event
}

func doShutdown(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	_, err := c.admin.Shutdown(ctx, &roxy_v0.ShutdownRequest{})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.Admin").
			Str("rpcMethod", "Shutdown").
			Err(err).
			Msg("RPC failed")
	}

	return event
}

func doHealthCheck(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	subsystemName := args[0]

	resp, err := c.health.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: subsystemName,
	})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "grpc.health.v1.Health").
			Str("rpcMethod", "Check").
			Err(err).
			Msg("RPC failed")
	}
	event = event.Stringer("status", resp.Status)

	return event
}

func doHealthWatch(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	subsystemName := args[0]

	wc, err := c.health.Watch(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: subsystemName,
	})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "grpc.health.v1.Health").
			Str("rpcMethod", "Watch").
			Err(err).
			Msg("RPC failed")
	}

	looping := true
	for looping {
		resp, err := wc.Recv()
		s, ok := status.FromError(err)
		switch {
		case err == nil:
			fmt.Println(resp.Status.String())
		case err == io.EOF:
			looping = false
		case errors.Is(err, context.Canceled):
			looping = false
		case ok && s.Code() == codes.Canceled:
			looping = false
		default:
			log.Logger.Fatal().
				Str("rpcService", "grpc.health.v1.Health").
				Str("rpcMethod", "Watch").
				Str("func", "WatchClient.Recv").
				Err(err).
				Msg("RPC failed")
		}
	}

	return event
}

func doSetHealth(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	subsystemName := args[0]
	isHealthyStr := args[1]

	isHealthy, err := misc.ParseBool(isHealthyStr)
	if err != nil {
		log.Logger.Fatal().
			Str("input", isHealthyStr).
			Err(err).
			Msg("invalid boolean")
	}

	_, err = c.admin.SetHealth(ctx, &roxy_v0.SetHealthRequest{
		SubsystemName: subsystemName,
		IsHealthy:     isHealthy,
	})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.Admin").
			Str("rpcMethod", "SetHealth").
			Str("subsystemName", subsystemName).
			Err(err).
			Msg("RPC failed")
	}

	return event
}

func doLookup(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	serviceName := args[0]
	shardStr := args[1]

	var shardNum uint32
	var hasShardNum bool
	if shardStr != "" {
		u64, err := strconv.ParseUint(shardStr, 10, 32)
		if err != nil {
			log.Logger.Fatal().
				Str("input", shardStr).
				Err(err).
				Msg("invalid ShardNumber")
		}
		shardNum, hasShardNum = uint32(u64), true
	}

	resp, err := c.atc.Lookup(ctx, &roxy_v0.LookupRequest{
		ServiceName:    serviceName,
		ShardNumber:    shardNum,
		HasShardNumber: hasShardNum,
	})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.AirTrafficControl").
			Str("rpcMethod", "Lookup").
			Str("serviceName", serviceName).
			Uint32("shardNum", shardNum).
			Bool("hasShardNum", hasShardNum).
			Err(err).
			Msg("RPC failed")
	}
	prettyPrint(os.Stdout, resp)

	return event
}

func doLookupClients(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	serviceName := args[0]
	shardStr := args[1]
	uniqueID := args[2]

	var shardNum uint32
	var hasShardNum bool
	if shardStr != "" {
		u64, err := strconv.ParseUint(shardStr, 10, 32)
		if err != nil {
			log.Logger.Fatal().
				Str("input", shardStr).
				Err(err).
				Msg("invalid ShardNumber")
		}
		shardNum, hasShardNum = uint32(u64), true
	}

	resp, err := c.atc.LookupClients(ctx, &roxy_v0.LookupClientsRequest{
		ServiceName:    serviceName,
		ShardNumber:    shardNum,
		HasShardNumber: hasShardNum,
		UniqueId:       uniqueID,
	})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.AirTrafficControl").
			Str("rpcMethod", "Lookup").
			Str("serviceName", serviceName).
			Uint32("shardNum", shardNum).
			Bool("hasShardNum", hasShardNum).
			Str("uniqueID", uniqueID).
			Err(err).
			Msg("RPC failed")
	}
	prettyPrint(os.Stdout, resp)

	return event
}

func doLookupServers(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	serviceName := args[0]
	shardStr := args[1]
	uniqueID := args[2]

	var shardNum uint32
	var hasShardNum bool
	if shardStr != "" {
		u64, err := strconv.ParseUint(shardStr, 10, 32)
		if err != nil {
			log.Logger.Fatal().
				Str("input", shardStr).
				Err(err).
				Msg("invalid ShardNumber")
		}
		shardNum, hasShardNum = uint32(u64), true
	}

	resp, err := c.atc.LookupServers(ctx, &roxy_v0.LookupServersRequest{
		ServiceName:    serviceName,
		ShardNumber:    shardNum,
		HasShardNumber: hasShardNum,
		UniqueId:       uniqueID,
	})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.AirTrafficControl").
			Str("rpcMethod", "Lookup").
			Str("serviceName", serviceName).
			Uint32("shardNum", shardNum).
			Bool("hasShardNum", hasShardNum).
			Str("uniqueID", uniqueID).
			Err(err).
			Msg("RPC failed")
	}
	prettyPrint(os.Stdout, resp)

	return event
}

func doFind(ctx context.Context, c clients, event *zerolog.Event, args []string) *zerolog.Event {
	serviceName := args[0]
	shardStr := args[1]

	var shardNum uint32
	var hasShardNum bool
	if shardStr != "" {
		u64, err := strconv.ParseUint(shardStr, 10, 32)
		if err != nil {
			log.Logger.Fatal().
				Str("input", shardStr).
				Err(err).
				Msg("invalid ShardNumber")
		}
		shardNum, hasShardNum = uint32(u64), true
	}

	resp, err := c.atc.Find(ctx, &roxy_v0.FindRequest{
		ServiceName: serviceName,
		ShardNumber: shardNum,
	})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.AirTrafficControl").
			Str("rpcMethod", "Find").
			Str("serviceName", serviceName).
			Uint32("shardNum", shardNum).
			Bool("hasShardNum", hasShardNum).
			Err(err).
			Msg("RPC failed")
	}
	prettyPrint(os.Stdout, resp)

	return event
}

func processCommandAndArgs() (fn commandFunc, cmd string, args []string) {
	args = getopt.Args()

	if len(args) == 0 {
		cmd = "help"
	} else {
		cmd = args[0]
	}

	if cmd == "help" {
		fmt.Print(helpText)
		os.Exit(0)
	}

	if alias, found := aliasMap[cmd]; found {
		cmd = alias
	}

	record, found := commandMap[cmd]
	if !found {
		log.Logger.Fatal().
			Str("cmd", cmd).
			Msg("unknown command")
	}

	actualNArgs := uint(len(args))
	if actualNArgs < record.minNArgs || actualNArgs > record.maxNArgs {
		log.Logger.Fatal().
			Str("cmd", cmd).
			Uint("minNArgs", record.minNArgs).
			Uint("maxNArgs", record.maxNArgs).
			Uint("actualNArgs", actualNArgs).
			Msg("wrong number of arguments")
	}

	tmp := args[1:]
	args = make([]string, record.maxNArgs-1)
	copy(args, tmp)

	fn = record.fn
	return
}

func prettyPrint(w io.Writer, v interface{}) {
	var buf bytes.Buffer
	buf.Grow(256)
	buf.WriteByte('\t')
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	e.SetIndent("\t", "  ")
	err := e.Encode(v)
	if err != nil {
		panic(err)
	}
	buf.WriteByte('\n')
	_, err = buf.WriteTo(w)
	if err != nil {
		panic(err)
	}
}
