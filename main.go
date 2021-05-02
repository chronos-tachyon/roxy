package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	getopt "github.com/pborman/getopt/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/journald"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/grpc/balancer/atcbalancer"
	"github.com/chronos-tachyon/roxy/roxypb"
)

var gDialer = net.Dialer{
	Timeout: 5 * time.Second,
}

var (
	gRootContext    context.Context
	gRootCancel     context.CancelFunc
	gLogger         *RotatingLogWriter
	gRef            Ref
	gAdminServer    *grpc.Server
	gPromServer     *http.Server
	gInsecureServer *http.Server
	gSecureServer   *http.Server
	gShutdownMu     sync.Mutex
	gAlreadyClosed  bool
	gAlreadyTermed  bool
	gShutdownCh     chan struct{}
)

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
	flagAdminNet    string = "unix"
	flagAdminAddr   string = "/var/opt/roxy/lib/admin.socket"
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
	getopt.FlagLong(&flagAdminNet, "admin-net", 0, "network for Admin gRPC interface")
	getopt.FlagLong(&flagAdminAddr, "admin-addr", 0, "address for Admin gRPC interface")
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

	if abs, err := processPath(flagConfig); err == nil {
		flagConfig = abs
	} else {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	if flagLogFile != "" {
		if abs, err := processPath(flagLogFile); err == nil {
			flagLogFile = abs
		} else {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
	}

	if strings.HasPrefix(flagPromNet, "unix") && flagPromAddr != "" && flagPromAddr[0] != '@' {
		if abs, err := processPath(flagPromAddr); err == nil {
			flagPromAddr = abs
		} else {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
	}

	if strings.HasPrefix(flagAdminNet, "unix") && flagAdminAddr != "" && flagAdminAddr[0] != '@' {
		if abs, err := processPath(flagAdminAddr); err == nil {
			flagAdminAddr = abs
		} else {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
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
			fmt.Fprintf(os.Stderr, "fatal: failed to open log file for append: %q: %v\n", flagLogFile, err)
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

	defer func() {
		if err := gRef.Close(); err != nil {
			log.Logger.Error().
				Err(err).
				Msg("close")
		}
	}()

	err := gRef.Load(flagConfig)
	if err != nil {
		log.Fatal().Err(err).Send()
		os.Exit(1)
	}

	gAdminServer = grpc.NewServer()
	roxypb.RegisterAdminServer(gAdminServer, AdminServer{})

	var promHandler http.Handler
	promHandler = promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			ErrorLog:            PromLoggerBridge{},
			Registry:            prometheus.DefaultRegisterer,
			MaxRequestsInFlight: 4,
			EnableOpenMetrics:   true,
		})
	promHandler = RootHandler{Ref: &gRef, Next: promHandler}
	gPromServer = &http.Server{
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
	gInsecureServer = &http.Server{
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
	gSecureServer = &http.Server{
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
			Str("proto", "admin").
			Err(err).
			Msg("failed to Listen")
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

	secureListener := &SecureListener{Ref: &gRef, Raw: secureListenerRaw}

	sdNotify("READY=1")

	gShutdownCh = make(chan struct{})

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := gAdminServer.Serve(adminListener)
		closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("proto", "admin").
				Err(err).
				Msg("failed to Serve")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := gPromServer.Serve(promListener)
		closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("proto", "prom").
				Err(err).
				Msg("failed to Serve")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := gInsecureServer.Serve(insecureListener)
		closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("proto", "http").
				Err(err).
				Msg("failed to Serve")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := gSecureServer.Serve(secureListener)
		closeShutdownCh()
		if isRealShutdownError(err) {
			log.Logger.Error().
				Str("proto", "https").
				Err(err).
				Msg("failed to Serve")
		}
	}()

	go func() {
		select {
		case <-gRootContext.Done():
			return
		case <-gShutdownCh:
			// pass
		}

		shutdown(true)

		t := time.NewTimer(5 * time.Second)
		select {
		case <-gRootContext.Done():
			t.Stop()
			return
		case <-t.C:
			// pass
		}

		shutdown(false)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for {
			sig := <-sigCh

			log.Logger.Info().
				Str("sig", sig.String()).
				Msg("got signal")

			switch sig {
			case syscall.SIGHUP:
				reload()

			case syscall.SIGINT:
				fallthrough
			case syscall.SIGTERM:
				shutdown(false)
			}
		}
	}()

	wg.Wait()
}

func closeShutdownCh() {
	gShutdownMu.Lock()
	if !gAlreadyClosed {
		gAlreadyClosed = true
		close(gShutdownCh)
	}
	gShutdownMu.Unlock()
}

func reload() error {
	var errs multierror.Error

	sdNotify("RELOADING=1")

	if gLogger != nil {
		if err := gLogger.Rotate(); err != nil {
			errs.Errors = append(errs.Errors, err)
			log.Logger.Error().
				Err(err).
				Msg("failed to rotate log file")
		}
	}

	if err := gRef.Load(flagConfig); err != nil {
		errs.Errors = append(errs.Errors, err)
		log.Logger.Error().
			Str("path", flagConfig).
			Err(err).
			Msg("failed to reload config file")
	}

	sdNotify("READY=1")

	return errs.ErrorOrNil()
}

func shutdown(graceful bool) error {
	gShutdownMu.Lock()
	alreadyTermed := gAlreadyTermed
	gAlreadyTermed = true
	gShutdownMu.Unlock()

	if alreadyTermed && graceful {
		return nil
	}

	if alreadyTermed {
		log.Logger.Warn().
			Msg("forcing dirty shutdown")
		gRootCancel()
	}

	sdNotify("STOPPING=1")

	var wg sync.WaitGroup
	errCh := make(chan error)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if alreadyTermed {
			gAdminServer.Stop()
		} else {
			go gAdminServer.GracefulStop()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var action string
		var err error
		if alreadyTermed {
			action = "Close"
			err = gSecureServer.Close()
		} else {
			action = "Shutdown"
			err = gSecureServer.Shutdown(gRootContext)
		}
		if isRealShutdownError(err) {
			errCh <- fmt.Errorf("failed to %s https server: %w", action, err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var action string
		var err error
		if alreadyTermed {
			action = "Close"
			err = gInsecureServer.Close()
		} else {
			action = "Shutdown"
			err = gInsecureServer.Shutdown(gRootContext)
		}
		if isRealShutdownError(err) {
			errCh <- fmt.Errorf("failed to %s http server: %w", action, err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var action string
		var err error
		if alreadyTermed {
			action = "Close"
			err = gPromServer.Close()
		} else {
			action = "Shutdown"
			err = gPromServer.Shutdown(gRootContext)
		}
		if isRealShutdownError(err) {
			errCh <- fmt.Errorf("failed to %s prom server: %w", action, err)
		}
	}()

	go func() {
		wg.Wait()
		if err := gRef.Close(); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	var errs multierror.Error
	for err := range errCh {
		errs.Errors = append(errs.Errors, err)
	}

	return errs.ErrorOrNil()
}

func isRealShutdownError(err error) bool {
	switch {
	case err == nil:
		return false

	case errors.Is(err, fs.ErrClosed):
		return false

	case errors.Is(err, net.ErrClosed):
		return false

	case errors.Is(err, http.ErrServerClosed):
		return false

	case errors.Is(err, context.Canceled):
		return false

	default:
		return true
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
