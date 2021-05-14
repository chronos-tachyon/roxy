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

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/roxypb"
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

	expanded, err := roxyutil.ExpandPath(flagConfig)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagConfig).
			Err(err).
			Msg("--config: failed to process path")
	}
	flagConfig = expanded

	var etcdConfig mainutil.EtcdConfig
	err = etcdConfig.Parse(flagEtcd)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagEtcd).
			Err(err).
			Msg("--etcd: failed to parse")
	}

	var adminListenConfig mainutil.ListenConfig
	err = adminListenConfig.Parse(flagListenAdmin)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagListenAdmin).
			Err(err).
			Msg("--listen-admin: failed to process path")
	}

	var promListenConfig mainutil.ListenConfig
	err = promListenConfig.Parse(flagListenProm)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagListenProm).
			Err(err).
			Msg("--listen-prom: failed to parse")
	}

	var atcListenConfig mainutil.ListenConfig
	err = atcListenConfig.Parse(flagListenATC)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagListenATC).
			Err(err).
			Msg("--listen-atc: failed to process path")
	}

	if !atcListenConfig.Enabled {
		log.Logger.Fatal().
			Str("input", flagListenATC).
			Msg("--listen-atc: required flag")
	}

	if net := atcListenConfig.Network; net != "tcp" && net != "tcp4" && net != "tcp6" {
		log.Logger.Fatal().
			Str("input", flagListenATC).
			Str("expected", "tcp").
			Str("actual", net).
			Msg("--listen-atc: TCP required")
	}

	atcAddr, err := misc.ParseTCPAddr(atcListenConfig.Address, "2987")
	if err != nil {
		log.Logger.Fatal().
			Str("input", atcListenConfig.Address).
			Err(err).
			Msg("--listen-atc: failed to parse TCP address")
	}
	atcListenConfig.Address = atcAddr.String()

	etcd, err := etcdConfig.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Interface("config", etcdConfig).
			Err(err).
			Msg("--etcd: failed to connect")
	}
	defer etcd.Close()

	var ref Ref
	ref.Init(flagConfig, atcAddr, etcd)

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

	adminServer := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(adminServer, &gHealthServer)
	roxypb.RegisterAdminServer(adminServer, AdminServer{})
	roxypb.RegisterAirTrafficControlServer(adminServer, &adminATCServer)

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
		ConnContext:       mainutil.MakeConnContextFunc("prom"),
	}

	atcServer := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(atcServer, &gHealthServer)
	roxypb.RegisterAirTrafficControlServer(atcServer, &mainATCServer)

	var adminListener net.Listener
	adminListener, err = adminListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "admin").
			Interface("config", adminListenConfig).
			Err(err).
			Msg("--listen-admin: failed to Listen")
	}

	var promListener net.Listener
	promListener, err = promListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "prom").
			Interface("config", promListenConfig).
			Err(err).
			Msg("--listen-prom: failed to Listen")
	}

	var atcListener net.Listener
	atcListener, err = atcListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "atc").
			Interface("config", atcListenConfig).
			Err(err).
			Msg("--listen-atc: failed to Listen")
	}

	gMultiServer.AddGRPCServer("admin", adminServer, adminListener)
	gMultiServer.AddHTTPServer("prom", promServer, promListener)
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
