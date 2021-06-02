// Command "roxy" is an Internet-facing HTTPS frontend proxy that uses ACME
// (Let's Encrypt et al.) to obtain TLS certificates.
//
// https://chronos-tachyon.github.io/roxy/
//
package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"sync"
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
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

var (
	gRef         Ref
	gMultiServer mainutil.MultiServer

	gDialer = net.Dialer{Timeout: 5 * time.Second}
)

var (
	flagConfig      string = defaultConfigFile
	flagListenAdmin string = "/var/opt/roxy/lib/admin.socket;net=unix"
	flagListenProm  string = "localhost:6800"
	flagUniqueFile  string = "/var/opt/roxy/lib/state/roxy.id"
)

func init() {
	getopt.SetParameters("")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagConfig, "config", 'c', "path to configuration file")
	getopt.FlagLong(&flagListenAdmin, "listen-admin", 'A', "listen config for Admin gRPC interface")
	getopt.FlagLong(&flagListenProm, "listen-prom", 'P', "listen config for Prometheus monitoring metrics")
	getopt.FlagLong(&flagUniqueFile, "unique-file", 'U', "file containing a unique ID for the ATC resolver")
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

	atcclient.SetUniqueFile(flagUniqueFile)

	roxyresolver.SetLogger(log.Logger.With().Str("package", "roxyresolver").Logger())

	var adminListenConfig mainutil.ListenConfig
	var adminServerOpts []grpc.ServerOption
	var promListenConfig mainutil.ListenConfig
	processFlags(
		&adminListenConfig,
		&adminServerOpts,
		&promListenConfig,
	)

	err := gRef.Load(ctx, flagConfig)
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Err(err).
			Msg("failed to load config file")
		os.Exit(1)
	}

	gMultiServer.OnExit(func(ctx context.Context) error {
		if err := gRef.Close(); err != nil {
			log.Logger.Error().
				Err(err).
				Msg("failed to close all handles")
			return err
		}
		return nil
	})

	adminServer := grpc.NewServer(adminServerOpts...)
	grpc_health_v1.RegisterHealthServer(adminServer, gMultiServer.HealthServer())
	roxy_v0.RegisterAdminServer(adminServer, AdminServer{})

	var promHandler http.Handler
	promHandler = promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			ErrorLog:            mainutil.PromLoggerBridge{},
			Registry:            prometheus.DefaultRegisterer,
			MaxRequestsInFlight: 4,
			EnableOpenMetrics:   true,
		})
	promHandler = RootHandler{Ref: &gRef, Next: promHandler}
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

	var insecureHandler http.Handler
	insecureHandler = &InsecureHandler{Next: nil}
	insecureHandler = RootHandler{Ref: &gRef, Next: insecureHandler}
	insecureServer := &http.Server{
		Handler:           insecureHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       mainutil.MakeBaseContextFunc(),
		ConnContext:       mainutil.MakeConnContextFunc(constants.SubsystemHTTP),
	}

	var secureHandler http.Handler
	secureHandler = SecureHandler{}
	secureHandler = RootHandler{Ref: &gRef, Next: secureHandler}
	secureServer := &http.Server{
		Handler:           secureHandler,
		ReadHeaderTimeout: 60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       mainutil.MakeBaseContextFunc(),
		ConnContext:       mainutil.MakeConnContextFunc(constants.SubsystemHTTPS),
	}

	adminListener, err := adminListenConfig.ListenNoTLS(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemAdmin).
			Err(err).
			Msg("--listen-admin: failed to Listen")
	}

	promListener, err := promListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemProm).
			Err(err).
			Msg("--listen-prom: failed to Listen")
	}

	insecureListener, err := net.Listen(constants.NetTCP, ":80")
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemHTTP).
			Err(err).
			Msg("failed to Listen")
	}

	secureListenerRaw, err := net.Listen(constants.NetTCP, ":443")
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemHTTPS).
			Err(err).
			Msg("failed to Listen")
	}

	secureListener := &SecureListener{Ref: &gRef, Raw: secureListenerRaw}

	gMultiServer.AddGRPCServer(constants.SubsystemAdmin, adminServer, adminListener)
	gMultiServer.AddHTTPServer(constants.SubsystemProm, promServer, promListener)
	gMultiServer.AddHTTPServer(constants.SubsystemHTTP, insecureServer, insecureListener)
	gMultiServer.AddHTTPServer(constants.SubsystemHTTPS, secureServer, secureListener)

	gMultiServer.OnReload(mainutil.RotateLogs)

	gMultiServer.OnReload(func(ctx context.Context) error {
		if err := gRef.Load(ctx, flagConfig); err != nil {
			log.Logger.Error().
				Str("path", flagConfig).
				Err(err).
				Msg("failed to reload config file")
			return err
		}
		return nil
	})

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

func processFlags(
	adminListenConfig *mainutil.ListenConfig,
	adminServerOpts *[]grpc.ServerOption,
	promListenConfig *mainutil.ListenConfig,
) {
	abs, err := roxyutil.ExpandPath(flagConfig)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagConfig).
			Err(err).
			Msg("--config: failed to process path")
	}
	flagConfig = abs

	err = adminListenConfig.Parse(flagListenAdmin)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemAdmin).
			Str("input", flagListenAdmin).
			Err(err).
			Msg("--listen-admin: failed to parse")
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
			Msg("--listen-prom: failed to parse")
	}

	log.Logger.Trace().
		Str("subsystem", constants.SubsystemProm).
		Interface("config", promListenConfig).
		Msg("ready")
}

type SecureListener struct {
	Ref       *Ref
	Raw       net.Listener
	mu        sync.Mutex
	savedImpl *Impl
	savedConf *tls.Config
}

func (l *SecureListener) Addr() net.Addr {
	return l.Raw.Addr()
}

func (l *SecureListener) Accept() (net.Conn, error) {
	rawConn, err := l.Raw.Accept()
	if err != nil {
		return nil, err
	}

	if tcpConn, ok := rawConn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(3 * time.Minute)
	}

	var tlsConfig *tls.Config
	impl := l.Ref.Get()
	l.mu.Lock()
	if impl == l.savedImpl {
		tlsConfig = l.savedConf
	} else {
		tlsConfig = impl.ACMEManager().TLSConfig()
		l.savedImpl = impl
		l.savedConf = tlsConfig
	}
	l.mu.Unlock()

	tlsConn := tls.Server(rawConn, tlsConfig)
	return tlsConn, nil
}

func (l *SecureListener) Close() error {
	return l.Raw.Close()
}

var _ net.Listener = (*SecureListener)(nil)
