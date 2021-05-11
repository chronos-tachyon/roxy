package main

import (
	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/roxypb"
)

var (
	flagAdminTarget string = "unix:/var/opt/roxy/lib/admin.socket"
)

func init() {
	getopt.SetParameters("<cmd> [<arg>...]")
	getopt.FlagLong(&flagAdminTarget, "server", 's', "address for Admin gRPC interface")
}

func main() {
	getopt.Parse()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	var cmd string
	if getopt.NArgs() == 0 {
		cmd = "ping"
	} else {
		cmd = getopt.Arg(0)
	}

	expectedNArgs := 1
	switch cmd {
	case "healthcheck":
		expectedNArgs = 2
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
	defer cc.Close()

	health := grpc_health_v1.NewHealthClient(cc)
	admin := roxypb.NewAdminClient(cc)

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
				Err(err).
				Msg("call to /grpc.health.v1.Health/Check failed")
		}
		event = event.Str("status", resp.Status.String())

	case "ping":
		_, err := admin.Ping(ctx, &roxypb.PingRequest{})
		if err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("call to /roxy.Admin/Ping failed")
		}

	case "reload":
		_, err := admin.Reload(ctx, &roxypb.ReloadRequest{})
		if err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("call to /roxy.Admin/Reload failed")
		}

	case "shutdown":
		_, err := admin.Shutdown(ctx, &roxypb.ShutdownRequest{})
		if err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("call to /roxy.Admin/Shutdown failed")
		}

	default:
		log.Logger.Fatal().
			Str("cmd", cmd).
			Msg("unknown command")
	}

	event.Msg("OK")
}
