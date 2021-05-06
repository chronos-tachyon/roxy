package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	getopt "github.com/pborman/getopt/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/roxypb"
)

var (
	gRef         Ref
	gMultiServer mainutil.MultiServer

	gDialer = net.Dialer{Timeout: 5 * time.Second}
)

var (
	flagConfig    string = defaultConfigFile
	flagPromNet   string = "tcp"
	flagPromAddr  string = "localhost:6800"
	flagAdminNet  string = "unix"
	flagAdminAddr string = "/var/opt/roxy/lib/admin.socket"
)

func init() {
	getopt.SetParameters("")

	getopt.FlagLong(&flagConfig, "config", 'c', "path to configuration file")
	getopt.FlagLong(&flagPromNet, "prometheus-net", 0, "network for Prometheus monitoring metrics")
	getopt.FlagLong(&flagPromAddr, "prometheus-addr", 0, "address for Prometheus monitoring metrics")
	getopt.FlagLong(&flagAdminNet, "admin-net", 0, "network for Admin gRPC interface")
	getopt.FlagLong(&flagAdminAddr, "admin-addr", 0, "address for Admin gRPC interface")
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

	abs, err := mainutil.ProcessPath(flagConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	flagConfig = abs

	if strings.HasPrefix(flagPromNet, "unix") && flagPromAddr != "" && flagPromAddr[0] != '@' {
		abs, err = mainutil.ProcessPath(flagPromAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
		flagPromAddr = abs
	}

	if strings.HasPrefix(flagAdminNet, "unix") && flagAdminAddr != "" && flagAdminAddr[0] != '@' {
		abs, err = mainutil.ProcessPath(flagAdminAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
		flagAdminAddr = abs
	}

	err = gRef.Load(ctx, flagConfig)
	if err != nil {
		log.Logger.Fatal().
			Str("path", flagConfig).
			Err(err).
			Msg("failed to load config file")
		os.Exit(1)
	}

	gMultiServer.OnExit(func() error {
		if err := gRef.Close(); err != nil {
			log.Logger.Error().
				Err(err).
				Msg("failed to close all handles")
			return err
		}
		return nil
	})

	adminServer := grpc.NewServer()
	roxypb.RegisterAdminServer(adminServer, AdminServer{})

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
		BaseContext:       MakeBaseContextFunc(),
		ConnContext:       MakeConnContextFunc("prom"),
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
		BaseContext:       MakeBaseContextFunc(),
		ConnContext:       MakeConnContextFunc("http"),
	}

	var secureHandler http.Handler
	secureHandler = SecureHandler{}
	secureHandler = RootHandler{Ref: &gRef, Next: secureHandler}
	secureServer := &http.Server{
		Handler:           secureHandler,
		ReadHeaderTimeout: 60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       MakeBaseContextFunc(),
		ConnContext:       MakeConnContextFunc("https"),
	}

	adminListener, err := net.Listen(flagAdminNet, flagAdminAddr)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "admin").
			Err(err).
			Msg("failed to Listen")
	}

	promListener, err := net.Listen(flagPromNet, flagPromAddr)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "prom").
			Err(err).
			Msg("failed to Listen")
	}

	insecureListener, err := net.Listen("tcp", ":80")
	if err != nil {
		log.Logger.Fatal().
			Str("server", "http").
			Err(err).
			Msg("failed to Listen")
	}

	secureListenerRaw, err := net.Listen("tcp", ":443")
	if err != nil {
		log.Logger.Fatal().
			Str("server", "https").
			Err(err).
			Msg("failed to Listen")
	}

	secureListener := &SecureListener{Ref: &gRef, Raw: secureListenerRaw}

	gMultiServer.AddGRPCServer("admin", adminServer, adminListener)
	gMultiServer.AddHTTPServer("prom", promServer, promListener)
	gMultiServer.AddHTTPServer("http", insecureServer, insecureListener)
	gMultiServer.AddHTTPServer("https", secureServer, secureListener)

	gMultiServer.OnReload(mainutil.RotateLogs)

	gMultiServer.OnReload(func() error {
		if err := gRef.Load(ctx, flagConfig); err != nil {
			log.Logger.Error().
				Str("path", flagConfig).
				Err(err).
				Msg("failed to reload config file")
			return err
		}
		return nil
	})

	gMultiServer.OnRun(func() {
		log.Logger.Info().
			Msg("Running")
	})

	gMultiServer.Run()

	log.Logger.Info().
		Msg("Exit")
}

func MakeBaseContextFunc() func(net.Listener) context.Context {
	return func(l net.Listener) context.Context {
		return mainutil.RootContext()
	}
}

func MakeConnContextFunc(name string) func(context.Context, net.Conn) context.Context {
	return func(ctx context.Context, c net.Conn) context.Context {
		cc := &ConnContext{
			Logger:     log.Logger.With().Str("server", name).Logger(),
			Proto:      name,
			LocalAddr:  c.LocalAddr(),
			RemoteAddr: c.RemoteAddr(),
		}
		ctx = WithConnContext(ctx, cc)
		ctx = cc.Logger.WithContext(ctx)
		cc.Context = ctx
		return ctx
	}
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
