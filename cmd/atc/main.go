// Command "atc" is the Roxy Air Traffic Controller, a piece of software which
// provides load-balanced routing of requests from ATC-aware clients to
// ATC-aware servers.
//
package main

import (
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
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

const defaultConfigFile = "/etc/opt/atc/config.json"

var (
	gMultiServer  mainutil.MultiServer
	gHealthServer mainutil.HealthServer
)

var (
	flagConfig      string = defaultConfigFile
	flagEtcd        string = "http://127.0.0.1:2379"
	flagListenAdmin string = "/var/opt/roxy/lib/atc.admin.socket;net=unix"
	flagListenProm  string = "localhost:6801"
	flagListenATC   string = "localhost:2987"
)

func init() {
	getopt.SetParameters("")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagConfig, "config", 'c', "path to configuration file")
	getopt.FlagLong(&flagEtcd, "etcd", 'E', "etcd config")
	getopt.FlagLong(&flagListenAdmin, "listen-admin", 'A', "listen config for Admin gRPC interface")
	getopt.FlagLong(&flagListenProm, "listen-prom", 'P', "listen config for Prometheus monitoring metrics")
	getopt.FlagLong(&flagListenATC, "listen-atc", 'L', "listen config for ATC gRPC interface")
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

	var etcdConfig mainutil.EtcdConfig
	var adminListenConfig mainutil.ListenConfig
	var adminServerOpts []grpc.ServerOption
	var promListenConfig mainutil.ListenConfig
	var grpcListenConfig mainutil.ListenConfig
	var grpcServerOpts []grpc.ServerOption
	var grpcAddr net.TCPAddr
	processFlags(
		&etcdConfig,
		&adminListenConfig,
		&adminServerOpts,
		&promListenConfig,
		&grpcListenConfig,
		&grpcServerOpts,
		&grpcAddr,
	)

	etcd, err := etcdConfig.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Interface("config", etcdConfig).
			Err(err).
			Msg("--etcd: failed to connect")
	}
	defer func() {
		_ = etcd.Close()
	}()

	var ref Ref
	ref.Init(flagConfig, &grpcAddr, etcd)

	err = ref.Load(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagConfig).
			Err(err).
			Msg("--config: failed to load")
	}

	ref.Flip()

	gMultiServer.OnReload(func() error {
		err := ref.Load(ctx)
		if err == nil {
			ref.Flip()
		}
		return err
	})

	mainATCServer := ATCServer{
		ref:   &ref,
		admin: false,
	}
	adminATCServer := ATCServer{
		ref:   &ref,
		admin: true,
	}

	adminServer := grpc.NewServer(adminServerOpts...)
	grpc_health_v1.RegisterHealthServer(adminServer, &gHealthServer)
	roxy_v0.RegisterAdminServer(adminServer, AdminServer{})
	roxy_v0.RegisterAirTrafficControlServer(adminServer, &adminATCServer)

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

	grpcServer := grpc.NewServer(grpcServerOpts...)
	grpc_health_v1.RegisterHealthServer(grpcServer, &gHealthServer)
	roxy_v0.RegisterAirTrafficControlServer(grpcServer, &mainATCServer)

	var adminListener net.Listener
	adminListener, err = adminListenConfig.ListenNoTLS(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemAdmin).
			Interface("config", adminListenConfig).
			Err(err).
			Msg("--listen-admin: failed to Listen")
	}

	var promListener net.Listener
	promListener, err = promListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemProm).
			Interface("config", promListenConfig).
			Err(err).
			Msg("--listen-prom: failed to Listen")
	}

	var grpcListener net.Listener
	grpcListener, err = grpcListenConfig.ListenNoTLS(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Interface("config", grpcListenConfig).
			Err(err).
			Msg("--listen-atc: failed to Listen")
	}

	gMultiServer.AddGRPCServer(constants.SubsystemAdmin, adminServer, adminListener)
	gMultiServer.AddHTTPServer(constants.SubsystemProm, promServer, promListener)
	gMultiServer.AddGRPCServer(constants.SubsystemGRPC, grpcServer, grpcListener)

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

func processFlags(
	etcdConfig *mainutil.EtcdConfig,
	adminListenConfig *mainutil.ListenConfig,
	adminServerOpts *[]grpc.ServerOption,
	promListenConfig *mainutil.ListenConfig,
	grpcListenConfig *mainutil.ListenConfig,
	grpcServerOpts *[]grpc.ServerOption,
	grpcAddr *net.TCPAddr,
) {
	expanded, err := roxyutil.ExpandPath(flagConfig)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagConfig).
			Err(err).
			Msg("--config: failed to process path")
	}
	flagConfig = expanded

	err = etcdConfig.Parse(flagEtcd)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagEtcd).
			Err(err).
			Msg("--etcd: failed to parse")
	}

	log.Logger.Trace().
		Str("subsystem", "etcd").
		Interface("config", etcdConfig).
		Msg("ready")

	err = adminListenConfig.Parse(flagListenAdmin)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemAdmin).
			Str("input", flagListenAdmin).
			Err(err).
			Msg("--listen-admin: failed to parse config")
	}

	*adminServerOpts, err = adminListenConfig.TLS.MakeGRPCServerOptions()
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemAdmin).
			Interface("config", adminListenConfig.TLS).
			Err(err).
			Msg("--listen-admin: failed to MakeGRPCServerOptions")
	}

	log.Logger.Trace().
		Str("subsystem", constants.SubsystemAdmin).
		Interface("config", adminListenConfig).
		Msg("ready")

	err = promListenConfig.Parse(flagListenProm)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemProm).
			Str("input", flagListenProm).
			Err(err).
			Msg("--listen-prom: failed to parse config")
	}

	log.Logger.Trace().
		Str("subsystem", constants.SubsystemProm).
		Interface("config", promListenConfig).
		Msg("ready")

	err = grpcListenConfig.Parse(flagListenATC)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Str("input", flagListenATC).
			Err(err).
			Msg("--listen-atc: failed to parse config")
	}

	if !grpcListenConfig.Enabled {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Str("input", flagListenATC).
			Msg("--listen-atc: required flag")
	}

	if !constants.IsNetTCP(grpcListenConfig.Network) {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Str("input", flagListenATC).
			Str("expected", constants.NetTCP).
			Str("actual", grpcListenConfig.Network).
			Msg("--listen-atc: TCP required")
	}

	addr, err := misc.ParseTCPAddr(grpcListenConfig.Address, constants.PortATC)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Str("input", grpcListenConfig.Address).
			Err(err).
			Msg("--listen-atc: failed to parse TCP address")
	}
	grpcListenConfig.Address = addr.String()
	*grpcAddr = *addr

	*grpcServerOpts, err = grpcListenConfig.TLS.MakeGRPCServerOptions()
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Interface("config", grpcListenConfig.TLS).
			Err(err).
			Msg("--listen-atc: failed to MakeGRPCServerOptions")
	}

	log.Logger.Trace().
		Str("subsystem", constants.SubsystemGRPC).
		Interface("config", grpcListenConfig).
		Msg("ready")
}
