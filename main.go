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
			log.Error().Err(err).Msg("close")
		}
	}()

	err := ref.Load(flagConfig)
	if err != nil {
		log.Fatal().Err(err).Send()
		os.Exit(1)
	}

	var (
		insecureHandler http.Handler
		insecureServer  *http.Server
	)

	insecureHandler = &InsecureHandler{Ref: &ref}
	insecureHandler = LoggingHandler{Next: insecureHandler}
	insecureHandler = BasicSecurityHandler{Next: insecureHandler}

	insecureServer = &http.Server{
		Addr:              ":80",
		Handler:           insecureHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(l net.Listener) context.Context {
			return gRootContext
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			logger := log.Logger.With().Str("proto", "http").Logger()
			ctx = logger.WithContext(ctx)
			ctx = context.WithValue(ctx, laddrKey{}, c.LocalAddr())
			ctx = context.WithValue(ctx, raddrKey{}, c.RemoteAddr())
			return ctx
		},
	}

	var (
		secureHandler http.Handler
		secureServer  *http.Server
	)

	secureHandler = ref.HTTPHandler()
	secureHandler = LoggingHandler{Next: secureHandler}
	secureHandler = BasicSecurityHandler{Next: secureHandler}

	secureServer = &http.Server{
		Addr:              ":443",
		Handler:           secureHandler,
		ReadHeaderTimeout: 60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(l net.Listener) context.Context {
			return gRootContext
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			logger := log.Logger.With().Str("proto", "https").Logger()
			ctx = logger.WithContext(ctx)
			ctx = context.WithValue(ctx, laddrKey{}, c.LocalAddr())
			ctx = context.WithValue(ctx, raddrKey{}, c.RemoteAddr())
			return ctx
		},
	}

	insecureListener, err := net.Listen("tcp", ":80")
	if err != nil {
		log.Logger.Fatal().
			Str("bind", ":80").
			Err(err).
			Msg("failed to listen")
	}

	secureListenerRaw, err := net.Listen("tcp", ":443")
	if err != nil {
		log.Logger.Fatal().
			Str("bind", ":443").
			Err(err).
			Msg("failed to listen")
	}

	secureListener := &SecureListener{Ref: &ref, Raw: secureListenerRaw}

	insecureDoneCh := make(chan struct{})
	secureDoneCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(insecureDoneCh)
		err := insecureServer.Serve(insecureListener)
		gRootCancel()
		if !isIgnoredServingError(err) {
			log.Error().
				Str("proto", "http").
				Err(err).
				Msg("failed to ListenAndServe")
		}
		go killServer(secureDoneCh, secureServer, secureListener)
	}()

	go func() {
		defer close(secureDoneCh)
		err := secureServer.Serve(secureListener)
		gRootCancel()
		if !isIgnoredServingError(err) {
			log.Error().
				Str("proto", "https").
				Err(err).
				Msg("failed to ListenAndServe")
		}
		go killServer(insecureDoneCh, insecureServer, insecureListener)
	}()

	var wg sync.WaitGroup
	wg.Add(2)
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

			log.Warn().
				Str("sig", sig.String()).
				Msg("got signal")

			switch sig {
			case syscall.SIGHUP:
				reload(&ref)

			case syscall.SIGINT:
				fallthrough
			case syscall.SIGTERM:
				if alreadyTermed {
					log.Warn().Msg("got second SIGINT/SIGTERM, forcing dirty shutdown")
					go killServer(insecureDoneCh, insecureServer, insecureListener)
					go killServer(secureDoneCh, secureServer, secureListener)
					return
				}

				alreadyTermed = true
				if err := secureServer.Shutdown(gRootContext); !isIgnoredServingError(err) {
					log.Error().
						Str("proto", "https").
						Err(err).
						Msg("failed to shutdown")
				}
				if err := insecureServer.Shutdown(gRootContext); !isIgnoredServingError(err) {
					log.Error().
						Str("proto", "http").
						Err(err).
						Msg("failed to shutdown")
				}
				gRootCancel()
			}
		}
	}()

	<-doneCh
}

func killServer(ch <-chan struct{}, server *http.Server, listener net.Listener) {
	err := listener.Close()
	if !isIgnoredServingError(err) {
		log.Logger.Warn().Err(err).Msg("failed to close listener")
	}

	t := time.NewTimer(5 * time.Second)
	select {
	case <-t.C:
		err := server.Close()
		if !isIgnoredServingError(err) {
			log.Logger.Warn().Err(err).Msg("failed to close server")
		}

	case <-ch:
		t.Stop()
	}
}

func reload(ref *Ref) {
	if gLogger != nil {
		if err := gLogger.Rotate(); err != nil {
			log.Error().
				Err(err).
				Msg("failed to rotate log file")
		}
	}

	if err := ref.Load(flagConfig); err != nil {
		log.Error().
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

type InsecureHandler struct {
	Ref          *Ref
	Next         http.Handler
	mu           sync.Mutex
	savedImpl    *Impl
	savedHandler http.Handler
}

func (h *InsecureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var next http.Handler
	impl := h.Ref.Get()
	h.mu.Lock()
	if impl == h.savedImpl {
		next = h.savedHandler
	} else {
		next = impl.ACMEManager().HTTPHandler(h.Next)
		h.savedImpl = impl
		h.savedHandler = next
	}
	h.mu.Unlock()
	next.ServeHTTP(w, r)
}

var _ http.Handler = (*InsecureHandler)(nil)

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
