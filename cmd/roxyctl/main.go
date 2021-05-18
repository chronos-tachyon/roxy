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
//	healthcheck name     check the health of the named subsystem(*)
//	set-health name y/n  set the health of the named subsystem
//
//	(*) At startup, the only available subsystem is the empty string, "".
//          The "set-health" command can create new named subsystems.
//
package main

import (
	"fmt"
	"os"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/health/grpc_health_v1"

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
	healthcheck <subsystem>
	set-health <subsystem> <value>
`

var (
	flagAdminTarget string = "unix:/var/opt/roxy/lib/admin.socket"
)

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

	var cmd string
	if getopt.NArgs() == 0 {
		cmd = "help"
	} else {
		cmd = getopt.Arg(0)
	}

	if cmd == "help" {
		fmt.Println(helpText)
		os.Exit(0)
	}

	expectedNArgs := 1
	switch cmd {
	case "healthcheck":
		expectedNArgs = 2

	case "set-health":
		expectedNArgs = 3
	}

	if getopt.NArgs() != expectedNArgs {
		log.Logger.Fatal().
			Int("expect", expectedNArgs).
			Int("actual", getopt.NArgs()).
			Msg("wrong number of arguments")
	}

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

	health := grpc_health_v1.NewHealthClient(cc)
	admin := roxy_v0.NewAdminClient(cc)

	event := log.Logger.Info()

	switch cmd {
	case "healthcheck":
		service := getopt.Arg(1)
		req := &grpc_health_v1.HealthCheckRequest{
			Service: service,
		}
		resp, err := health.Check(ctx, req)
		if err != nil {
			log.Logger.Fatal().
				Str("rpcService", "grpc.health.v1.Health").
				Str("rpcMethod", "Check").
				Err(err).
				Msg("RPC failed")
		}
		event = event.Str("status", resp.Status.String())

	case "ping":
		_, err := admin.Ping(ctx, &roxy_v0.PingRequest{})
		if err != nil {
			log.Logger.Fatal().
				Str("rpcService", "roxy.v0.Admin").
				Str("rpcMethod", "Ping").
				Err(err).
				Msg("RPC failed")
		}

	case "reload":
		_, err := admin.Reload(ctx, &roxy_v0.ReloadRequest{})
		if err != nil {
			log.Logger.Fatal().
				Str("rpcService", "roxy.v0.Admin").
				Str("rpcMethod", "Reload").
				Err(err).
				Msg("RPC failed")
		}

	case "shutdown":
		_, err := admin.Shutdown(ctx, &roxy_v0.ShutdownRequest{})
		if err != nil {
			log.Logger.Fatal().
				Str("rpcService", "roxy.v0.Admin").
				Str("rpcMethod", "Shutdown").
				Err(err).
				Msg("RPC failed")
		}

	case "set-health":
		subsystemName := getopt.Arg(1)
		isHealthy, err := misc.ParseBool(getopt.Arg(2))
		if err != nil {
			log.Logger.Fatal().
				Str("input", getopt.Arg(2)).
				Err(err).
				Msg("invalid boolean")
		}
		req := &roxy_v0.SetHealthRequest{
			SubsystemName: subsystemName,
			IsHealthy:     isHealthy,
		}
		_, err = admin.SetHealth(ctx, req)
		if err != nil {
			log.Logger.Fatal().
				Str("rpcService", "roxy.v0.Admin").
				Str("rpcMethod", "SetHealth").
				Str("subsystemName", subsystemName).
				Err(err).
				Msg("RPC failed")
		}

	default:
		log.Logger.Fatal().
			Str("cmd", cmd).
			Msg("unknown command")
	}

	event.Msg("OK")
}
