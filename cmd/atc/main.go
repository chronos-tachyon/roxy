package main

import (
	"net"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/roxypb"
)

const defaultConfigFile = "/etc/opt/roxy/atc.json"

var (
	gMultiServer  mainutil.MultiServer
	gHealthServer mainutil.HealthServer
	gATCServer    ATCServer
)

var (
	flagConfig string = defaultConfigFile
	flagListenATC    string = "127.0.0.1:2987"
	flagListenAdmin  string = "/var/opt/roxy/lib/atc.admin.socket;net=unix"
)

func init() {
	getopt.SetParameters("")

	getopt.FlagLong(&flagConfig, "config", 'c', "path to configuration file")
	getopt.FlagLong(&flagListenATC, "listen-atc", 'L', "ATC gRPC interface listen config")
	getopt.FlagLong(&flagListenAdmin, "listen-admin", 'A', "Admin gRPC interface listen config")
}

func main() {
	// Allocate some dummy memory to make the GC less aggressive.
	// https://blog.twitch.tv/en/2019/04/10/go-memory-ballast-how-i-learnt-to-stop-worrying-and-love-the-heap-26c2462549a2/
	gcBallast := make([]byte, 1<<30) // 1 GiB
	_ = gcBallast

	getopt.Parse()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	roxyresolver.SetLogger(log.Logger.With().Str("package", "roxyresolver").Logger())

	expanded, err := mainutil.ProcessPath(flagConfig)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagConfig).
			Err(err).
			Msg("--config: failed to process path")
	}
	flagConfig = expanded

	var atcListenConfig mainutil.ListenConfig
	err = atcListenConfig.Parse(flagListenATC)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagListenATC).
			Err(err).
			Msg("--listen-atc: failed to process path")
	}

	var adminListenConfig mainutil.ListenConfig
	err = adminListenConfig.Parse(flagListenAdmin)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagListenAdmin).
			Err(err).
			Msg("--listen-admin: failed to process path")
	}

	adminServer := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(adminServer, &gHealthServer)
	roxypb.RegisterAdminServer(adminServer, AdminServer{})

	var adminListener net.Listener
	adminListener, err = adminListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "admin").
			Interface("config", adminListenConfig).
			Err(err).
			Msg("failed to Listen")
	}

	gMultiServer.AddGRPCServer("admin", adminServer, adminListener)

	atcServer := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(atcServer, &gHealthServer)
	roxypb.RegisterAirTrafficControlServer(atcServer, &gATCServer)

	var atcListener net.Listener
	atcListener, err = atcListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "atc").
			Interface("config", atcListenConfig).
			Err(err).
			Msg("failed to Listen")
	}

	gMultiServer.AddGRPCServer("atc", atcServer, atcListener)

	gMultiServer.OnReload(mainutil.RotateLogs)

	gMultiServer.OnRun(func() {
		log.Logger.Info().
			Msg("Running")
	})

	gHealthServer.Set("", true)
	gMultiServer.OnShutdown(func(alreadyTermed bool) error {
		gHealthServer.Stop()
		return nil
	})

	gMultiServer.Run()

	log.Logger.Info().
		Msg("Exit")
}
