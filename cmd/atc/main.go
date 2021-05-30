// Command "atc" is the Roxy Air Traffic Controller, a piece of software which
// provides load-balanced routing of requests from ATC-aware clients to
// ATC-aware servers.
//
package main

import (
	"context"
	"net"
	"net/http"
	"time"

	getopt "github.com/pborman/getopt/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

var gMultiServer mainutil.MultiServer

var (
	flagConfig string = "/etc/opt/atc/global.json"
)

func init() {
	getopt.SetParameters("")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagConfig, "config", 'c', "path to configuration file")
}

func main() {
	// Allocate some dummy memory to make the GC less aggressive.
	// https://blog.twitch.tv/en/2019/04/10/go-memory-ballast-how-i-learnt-to-stop-worrying-and-love-the-heap-26c2462549a2/
	gcBallast := make([]byte, 1<<30) // 1 GiB
	_ = gcBallast

	getopt.Parse()

	mainutil.InitVersion()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	roxyresolver.SetLogger(log.Logger.With().Str("package", "roxyresolver").Logger())

	cfg, err := LoadGlobalConfigFile()
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Err(err).
			Msg("--config: failed to load")
	}

	adminServerOpts, err := cfg.ListenAdmin.TLS.MakeGRPCServerOptions()
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Str("subsystem", constants.SubsystemAdmin).
			Interface("config", cfg.ListenAdmin.TLS).
			Err(err).
			Msg("failed to MakeGRPCServerOptions")
	}

	grpcServerOpts, err := cfg.ListenGRPC.TLS.MakeGRPCServerOptions()
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Str("subsystem", constants.SubsystemGRPC).
			Interface("config", cfg.ListenGRPC.TLS).
			Err(err).
			Msg("failed to MakeGRPCServerOptions")
	}

	zkConn, err := cfg.ZK.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Str("subsystem", "zk").
			Interface("config", cfg.ZK).
			Err(err).
			Msg("failed to connect")
	}
	if zkConn != nil {
		gMultiServer.OnExit(func(context.Context) error {
			zkConn.Close()
			return nil
		})

		ctx = roxyresolver.WithZKConn(ctx, zkConn)
	}

	etcd, err := cfg.Etcd.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Str("subsystem", "etcd").
			Interface("config", cfg.Etcd).
			Err(err).
			Msg("failed to connect")
	}
	if etcd != nil {
		gMultiServer.OnExit(func(context.Context) error {
			return etcd.Close()
		})

		ctx = roxyresolver.WithEtcdV3Client(ctx, etcd)
	}

	var ref Ref
	ref.Init(cfg, zkConn, etcd)

	err = ref.Load(ctx, 0, 0)
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Err(err).
			Msg("--config: failed to load")
	}

	err = ref.Flip(ctx, 0)
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Err(err).
			Msg("--config: failed to activate")
	}

	err = ref.Commit(ctx, 0)
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Err(err).
			Msg("--config: failed to commit")
	}

	promHandler := promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			ErrorLog:            mainutil.PromLoggerBridge{},
			Registry:            prometheus.DefaultRegisterer,
			MaxRequestsInFlight: 4,
			EnableOpenMetrics:   true,
		})
	promServer := &http.Server{
		Handler:           promHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       mainutil.MakeBaseContextFunc(),
		ConnContext:       mainutil.MakeConnContextFunc(constants.SubsystemProm),
	}

	adminServer := grpc.NewServer(adminServerOpts...)
	grpc_health_v1.RegisterHealthServer(adminServer, gMultiServer.HealthServer())
	roxy_v0.RegisterAdminServer(adminServer, AdminServer{ref: &ref})
	roxy_v0.RegisterAirTrafficControlServer(adminServer, &ATCServer{ref: &ref, admin: true})

	grpcServer := grpc.NewServer(grpcServerOpts...)
	grpc_health_v1.RegisterHealthServer(grpcServer, gMultiServer.HealthServer())
	roxy_v0.RegisterAirTrafficControlServer(grpcServer, &ATCServer{ref: &ref, admin: false})

	var promListener net.Listener
	promListener, err = cfg.ListenProm.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemProm).
			Interface("config", cfg.ListenProm).
			Err(err).
			Msg("failed to Listen")
	}

	var adminListener net.Listener
	adminListener, err = cfg.ListenAdmin.ListenNoTLS(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemAdmin).
			Interface("config", cfg.ListenAdmin).
			Err(err).
			Msg("failed to Listen")
	}

	var grpcListener net.Listener
	grpcListener, err = cfg.ListenGRPC.ListenNoTLS(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Interface("config", cfg.ListenGRPC).
			Err(err).
			Msg("failed to Listen")
	}

	gMultiServer.AddHTTPServer(constants.SubsystemProm, promServer, promListener)
	gMultiServer.AddGRPCServer(constants.SubsystemAdmin, adminServer, adminListener)
	gMultiServer.AddGRPCServer(constants.SubsystemGRPC, grpcServer, grpcListener)

	gMultiServer.OnReload(mainutil.RotateLogs)

	gMultiServer.SetHealth("", true)
	gMultiServer.WatchHealth(func(subsystemName string, isHealthy bool, isStopped bool) {
		if subsystemName == "" {
			atcclient.SetIsServing(isHealthy)
		}
	})

	err = gMultiServer.Run(ctx)
	if err != nil {
		log.Logger.Error().
			Err(err).
			Msg("Run")
		return
	}
}
