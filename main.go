package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"time"

	getopt "github.com/pborman/getopt/v2"
	zerolog "github.com/rs/zerolog"
	journald "github.com/rs/zerolog/journald"
	log "github.com/rs/zerolog/log"
	acme "golang.org/x/crypto/acme"
	autocert "golang.org/x/crypto/acme/autocert"
	errgroup "golang.org/x/sync/errgroup"
)

var gDialer = net.Dialer{
	Timeout: 5 * time.Second,
}

var gLogFile *os.File

var gLogFileRotateCallback = func() error {
	return nil
}

var (
	flagConfig      string
	flagDebug       bool
	flagLogStderr   bool
	flagLogJournald bool
	flagLogFile     string
)

func init() {
	getopt.SetParameters("")

	getopt.FlagLong(&flagConfig, "config", 'c', "path to configuration file")
	getopt.FlagLong(&flagDebug, "debug", 'd', "enable debug logging")
	getopt.FlagLong(&flagLogStderr, "log-stderr", 'S', "log JSON to stderr")
	getopt.FlagLong(&flagLogJournald, "log-journald", 'J', "log to journald")
	getopt.FlagLong(&flagLogFile, "log-file", 'l', "log JSON to file")
}

func main() {
	// Allocate some dummy memory to make the GC less aggressive.
	// https://blog.twitch.tv/en/2019/04/10/go-memory-ballast-how-i-learnt-to-stop-worrying-and-love-the-heap-26c2462549a2/
	gcBallast := make([]byte, 1<<30) // 1 GiB
	_ = gcBallast

	getopt.Parse()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.DurationFieldUnit = time.Second
	zerolog.DurationFieldInteger = false
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if flagDebug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
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
		if gLogFile != nil {
			gLogFile.Close()
		}
	}()

	switch {
	case flagLogStderr:
		// do nothing

	case flagLogJournald:
		log.Logger = log.Output(journald.NewJournalDWriter())

	case flagLogFile != "":
		var err error
		gLogFile, err = os.OpenFile(flagLogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: failed to open log file for append: %q: %v", flagLogFile, err)
			os.Exit(1)
		}
		log.Logger = log.Output(gLogFile)

	default:
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	stdlog.SetFlags(0)
	stdlog.SetOutput(log.Logger)

	rootContext := context.Background()

	var roxy Ref
	err := roxy.Load(flagConfig)
	if err != nil {
		log.Fatal().Err(err).Send()
		os.Exit(1)
	}

	acmeClient := &acme.Client{
		DirectoryURL: autocert.DefaultACMEDirectory,
	}

	acmeManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Client:     acmeClient,
		Cache:      roxy.Cache(),
		HostPolicy: roxy.HostPolicy(),
	}

	var (
		insecureHandler http.Handler
		insecureServer  http.Server
	)

	insecureHandler = acmeManager.HTTPHandler(nil)

	insecureHandler = LoggingHandler{
		RootLogger: &log.Logger,
		Service:    "http",
		Next:       insecureHandler,
	}

	insecureHandler = BasicSecurityHandler{
		Ref:  &roxy,
		Next: insecureHandler,
	}

	insecureServer = http.Server{
		Addr:              ":80",
		Handler:           insecureHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(l net.Listener) context.Context {
			return rootContext
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			logger := log.Logger.With().
				Str("proto", "http").
				Str("laddr", c.LocalAddr().String()).
				Str("raddr", c.RemoteAddr().String()).
				Logger()
			return logger.WithContext(ctx)
		},
	}

	var (
		secureHandler http.Handler
		secureServer  http.Server
	)

	secureHandler = roxy.Handler()

	secureHandler = LoggingHandler{
		RootLogger: &log.Logger,
		Service:    "https",
		Next:       secureHandler,
	}

	secureHandler = BasicSecurityHandler{
		Ref:  &roxy,
		Next: secureHandler,
	}

	secureServer = http.Server{
		Addr:              ":443",
		Handler:           secureHandler,
		TLSConfig:         acmeManager.TLSConfig(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(l net.Listener) context.Context {
			return rootContext
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			logger := log.Logger.With().
				Str("proto", "https").
				Str("laddr", c.LocalAddr().String()).
				Str("raddr", c.RemoteAddr().String()).
				Logger()
			return logger.WithContext(ctx)
		},
	}

	g, groupContext := errgroup.WithContext(rootContext)

	_ = groupContext

	g.Go(func() error {
		err := insecureServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return err
	})

	g.Go(func() error {
		err := secureServer.Serve(acmeManager.Listener())
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return err
	})

	err = g.Wait()
	if err != nil {
		log.Fatal().Err(err).Send()
	}
}
