// Command "roxyctl" is the admin CLI for "roxy".
//
// Usage:
//
//	roxyctl [<flags>] <cmd> [<args>...]
//
// Flags:
//
//	-V, --version        print version and exit
//	-s, --server=addr    Address of the Roxy admin port
//	     [default: "unix:/var/opt/roxy/lib/admin.socket"]
//	-J, --log-journald   log to journald
//	-l, --log-file=path  log JSON to file
//	-S, --log-stderr     log JSON to stderr
//	-v, --verbose        enable debug logging
//	-d, --debug          enable debug and trace logging
//
// Commands:
//
//	help                 list available commands
//	ping                 check that Roxy is running
//	reload               tell Roxy to reload its config and to rotate its log file
//	shutdown             tell Roxy to exit
//	healthcheck [name]   check the health of the named subsystem(*)
//	healthwatch [name]   watch the health of the named subsystem(*)
//	set-health name y/n  set the health of the named subsystem
//
//	(*) At startup, the only available subsystem is the empty string, "".
//          The "set-health" command can create new named subsystems.
//
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
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

const helpText = `roxyctl [<flags>] <cmd> [<arg>...]
Commands available:
	help
	ping
	reload
	shutdown
	healthcheck [<subsystem>]
	healthwatch [<subsystem>]
	set-health <subsystem> <value>
`

var (
	flagAdminTarget string = "unix:/var/opt/roxy/lib/admin.socket"
)

const (
	cmdPing        = "ping"
	cmdReload      = "reload"
	cmdShutdown    = "shutdown"
	cmdHealthCheck = "healthcheck"
	cmdHealthWatch = "healthwatch"
	cmdSetHealth   = "set-health"
)

type clients struct {
	cc     *grpc.ClientConn
	health grpc_health_v1.HealthClient
	admin  roxy_v0.AdminClient
}

type commandFunc func(context.Context, clients, *zerolog.Event, []string) *zerolog.Event

type commandRecord struct {
	fn       commandFunc
	minNArgs uint
	maxNArgs uint
}

var aliasMap = map[string]string{
	"health":       cmdHealthCheck,
	"health-check": cmdHealthCheck,
	"check-health": cmdHealthCheck,
	"health-watch": cmdHealthWatch,
	"watch-health": cmdHealthWatch,
}

var commandMap = map[string]commandRecord{
	cmdPing: {
		fn:       doPing,
		minNArgs: 1,
		maxNArgs: 1,
	},
	cmdReload: {
		fn:       doReload,
		minNArgs: 1,
		maxNArgs: 1,
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
	_, err := c.admin.Reload(ctx, &roxy_v0.ReloadRequest{})
	if err != nil {
		log.Logger.Fatal().
			Str("rpcService", "roxy.v0.Admin").
			Str("rpcMethod", "Reload").
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
	event = event.Str("status", resp.Status.String())

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

func processCommandAndArgs() (fn commandFunc, cmd string, args []string) {
	args = getopt.Args()

	if len(args) == 0 {
		cmd = "help"
	} else {
		cmd = args[0]
	}

	if cmd == "help" {
		fmt.Println(helpText)
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
