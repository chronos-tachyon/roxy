package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	getopt "github.com/pborman/getopt/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/journald"
	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/grpc/balancer/atcbalancer"
)

var gDialer = net.Dialer{
	Timeout: 5 * time.Second,
}

var gLogger *RotatingLogWriter

var gRootContext context.Context
var gRootCancel context.CancelFunc

var (
	flagConfig      string = defaultConfigFile
	flagDebug       bool
	flagTrace       bool
	flagLogStderr   bool
	flagLogJournald bool
	flagLogFile     string
	flagVersion     bool
	flagPromNet     string = "tcp"
	flagPromAddr    string = "localhost:6800"
)

func init() {
	getopt.SetParameters("")

	getopt.FlagLong(&flagConfig, "config", 'c', "path to configuration file")
	getopt.FlagLong(&flagDebug, "debug", 'd', "enable debug logging")
	getopt.FlagLong(&flagTrace, "trace", 'D', "enable debug and trace logging")
	getopt.FlagLong(&flagLogStderr, "log-stderr", 'S', "log JSON to stderr")
	getopt.FlagLong(&flagLogJournald, "log-journald", 'J', "log to journald")
	getopt.FlagLong(&flagLogFile, "log-file", 'l', "log JSON to file")
	getopt.FlagLong(&flagVersion, "version", 'V', "print version and exit")
	getopt.FlagLong(&flagPromNet, "prometheus-net", 0, "network for Prometheus monitoring metrics")
	getopt.FlagLong(&flagPromAddr, "prometheus-addr", 0, "address for Prometheus monitoring metrics")
}

func main() {
	// Allocate some dummy memory to make the GC less aggressive.
	// https://blog.twitch.tv/en/2019/04/10/go-memory-ballast-how-i-learnt-to-stop-worrying-and-love-the-heap-26c2462549a2/
	gcBallast := make([]byte, 1<<30) // 1 GiB
	_ = gcBallast

	getopt.Parse()

	if flagVersion {
		fmt.Println(Version())
		os.Exit(0)
	}
	if flagLogStderr && flagLogJournald {
		fmt.Fprintln(os.Stderr, "fatal: flags '-S'/'--log-stderr' and '-J'/'--log-journald' are mutually exclusive")
		os.Exit(1)
	}
	if flagLogStderr && flagLogFile != "" {
		fmt.Fprintln(os.Stderr, "fatal: flags '-S'/'--log-stderr' and '-l'/'--log-file' are mutually exclusive")
		os.Exit(1)
	}
	if flagLogJournald && flagLogFile != "" {
		fmt.Fprintln(os.Stderr, "fatal: flags '-J'/'--log-journald' and '-l'/'--log-file' are mutually exclusive")
		os.Exit(1)
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.DurationFieldUnit = time.Second
	zerolog.DurationFieldInteger = false
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if flagDebug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	if flagTrace {
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}

	defer func() {
		if gLogger != nil {
			gLogger.Close()
		}
	}()

	switch {
	case flagLogStderr:
		// do nothing

	case flagLogJournald:
		log.Logger = log.Output(journald.NewJournalDWriter())

	case flagLogFile != "":
		var err error
		gLogger, err = NewRotatingLogWriter(flagLogFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: failed to open log file for append: %q: %v", flagLogFile, err)
			os.Exit(1)
		}
		log.Logger = log.Output(gLogger)

	default:
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	stdlog.SetFlags(0)
	stdlog.SetOutput(log.Logger)

	atcbalancer.SetLogger(log.Logger.With().Str("package", "atcbalancer").Logger())

	gRootContext = context.Background()
	gRootContext, gRootCancel = context.WithCancel(gRootContext)
	defer gRootCancel()

	var ref Ref
	defer func() {
		if err := ref.Close(); err != nil {
			log.Logger.Error().
				Err(err).
				Msg("close")
		}
	}()

	err := ref.Load(flagConfig)
	if err != nil {
		log.Fatal().Err(err).Send()
		os.Exit(1)
	}

	var promHandler http.Handler
	promHandler = promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			ErrorLog:            PromLoggerBridge{},
			Registry:            prometheus.DefaultRegisterer,
			MaxRequestsInFlight: 4,
			EnableOpenMetrics:   true,
		})
	promHandler = RootHandler{Ref: &ref, Next: promHandler}
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
	insecureHandler = RootHandler{Ref: &ref, Next: insecureHandler}
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
	secureHandler = RootHandler{Ref: &ref, Next: secureHandler}
	secureServer := &http.Server{
		Handler:           secureHandler,
		ReadHeaderTimeout: 60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       MakeBaseContextFunc(),
		ConnContext:       MakeConnContextFunc("https"),
	}

	promListener, err := net.Listen(flagPromNet, flagPromAddr)
	if err != nil {
		log.Logger.Fatal().
			Str("proto", "prom").
			Err(err).
			Msg("failed to Listen")
	}

	insecureListener, err := net.Listen("tcp", ":80")
	if err != nil {
		log.Logger.Fatal().
			Str("proto", "http").
			Err(err).
			Msg("failed to Listen")
	}

	secureListenerRaw, err := net.Listen("tcp", ":443")
	if err != nil {
		log.Logger.Fatal().
			Str("proto", "https").
			Err(err).
			Msg("failed to Listen")
	}

	secureListener := &SecureListener{Ref: &ref, Raw: secureListenerRaw}

	promDoneCh := make(chan struct{})
	insecureDoneCh := make(chan struct{})
	secureDoneCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(promDoneCh)
		err := promServer.Serve(promListener)
		gRootCancel()
		if !isIgnoredServingError(err) {
			log.Logger.Error().
				Str("proto", "prom").
				Err(err).
				Msg("failed to Serve")
		}
		go killServer("http", insecureDoneCh, insecureServer, insecureListener)
		go killServer("https", secureDoneCh, secureServer, secureListener)
	}()

	go func() {
		defer close(insecureDoneCh)
		err := insecureServer.Serve(insecureListener)
		gRootCancel()
		if !isIgnoredServingError(err) {
			log.Logger.Error().
				Str("proto", "http").
				Err(err).
				Msg("failed to Serve")
		}
		go killServer("prom", promDoneCh, promServer, promListener)
		go killServer("https", secureDoneCh, secureServer, secureListener)
	}()

	go func() {
		defer close(secureDoneCh)
		err := secureServer.Serve(secureListener)
		gRootCancel()
		if !isIgnoredServingError(err) {
			log.Logger.Error().
				Str("proto", "https").
				Err(err).
				Msg("failed to Serve")
		}
		go killServer("prom", promDoneCh, promServer, promListener)
		go killServer("http", insecureDoneCh, insecureServer, insecureListener)
	}()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-promDoneCh
	}()
	go func() {
		defer wg.Done()
		<-insecureDoneCh
	}()
	go func() {
		defer wg.Done()
		<-secureDoneCh
	}()
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		defer signal.Stop(sigCh)
		alreadyTermed := false
		for {
			sig := <-sigCh

			log.Logger.Warn().
				Str("sig", sig.String()).
				Msg("got signal")

			switch sig {
			case syscall.SIGHUP:
				reload(&ref)

			case syscall.SIGINT:
				fallthrough
			case syscall.SIGTERM:
				if alreadyTermed {
					log.Logger.Warn().
						Msg("got second SIGINT/SIGTERM, forcing dirty shutdown")
					go killServer("prom", promDoneCh, promServer, promListener)
					go killServer("http", insecureDoneCh, insecureServer, insecureListener)
					go killServer("https", secureDoneCh, secureServer, secureListener)
					return
				}

				alreadyTermed = true
				if err := secureServer.Shutdown(gRootContext); !isIgnoredServingError(err) {
					log.Logger.Error().
						Str("proto", "https").
						Err(err).
						Msg("failed to Shutdown")
				}
				if err := insecureServer.Shutdown(gRootContext); !isIgnoredServingError(err) {
					log.Logger.Error().
						Str("proto", "http").
						Err(err).
						Msg("failed to Shutdown")
				}
				if err := promServer.Shutdown(gRootContext); !isIgnoredServingError(err) {
					log.Logger.Error().
						Str("proto", "prom").
						Err(err).
						Msg("failed to Shutdown")
				}
				gRootCancel()
			}
		}
	}()

	<-doneCh
}

func killServer(proto string, ch <-chan struct{}, server *http.Server, listener net.Listener) {
	err := listener.Close()
	if !isIgnoredServingError(err) {
		log.Logger.Warn().
			Str("proto", proto).
			Err(err).
			Msg("failed to Close listener")
	}

	t := time.NewTimer(5 * time.Second)
	select {
	case <-t.C:
		err := server.Close()
		if !isIgnoredServingError(err) {
			log.Logger.Warn().
				Str("proto", proto).
				Err(err).
				Msg("failed to Close server")
		}

	case <-ch:
		t.Stop()
	}
}

func reload(ref *Ref) {
	if gLogger != nil {
		if err := gLogger.Rotate(); err != nil {
			log.Logger.Error().
				Err(err).
				Msg("failed to rotate log file")
		}
	}

	if err := ref.Load(flagConfig); err != nil {
		log.Logger.Error().
			Str("path", flagConfig).
			Err(err).
			Msg("failed to reload config file")
	}
}

func isIgnoredServingError(err error) bool {
	switch {
	case err == nil:
		return true

	case errors.Is(err, net.ErrClosed):
		return true

	case errors.Is(err, http.ErrServerClosed):
		return true

	case errors.Is(err, context.Canceled):
		return true

	default:
		return false
	}
}

func MakeBaseContextFunc() func(net.Listener) context.Context {
	return func(l net.Listener) context.Context {
		return gRootContext
	}
}

func MakeConnContextFunc(proto string) func(context.Context, net.Conn) context.Context {
	return func(ctx context.Context, c net.Conn) context.Context {
		cc := &ConnContext{
			Logger:     log.Logger.With().Str("proto", proto).Logger(),
			Proto:      proto,
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
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(3 * time.Minute)
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
