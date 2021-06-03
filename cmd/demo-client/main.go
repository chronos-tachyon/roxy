// Command "demo-client" is a demonstration of the "roxyresolver" and
// "atcclient" libraries.
//
package main

import (
	"io"
	"time"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

var (
	flagATC        string = ""
	flagWeb        string = ""
	flagUniqueFile string = "/var/opt/roxy/lib/state/demo-client.id"
	flagDelay      time.Duration = 1 * time.Second
	flagIterations int = 5
)

func init() {
	getopt.SetParameters("")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagATC, "atc", 'A', "ATC server configuration")
	getopt.FlagLong(&flagWeb, "web", 'W', "gRPC roxy.v0.Web server to connect to")
	getopt.FlagLong(&flagUniqueFile, "unique-file", 'U', "file containing a unique ID for the ATC announcer")
	getopt.FlagLong(&flagDelay, "delay", 0, "delay between iterations")
	getopt.FlagLong(&flagIterations, "iterations", 'n', "number of iterations")
}

func main() {
	getopt.Parse()

	mainutil.InitVersion()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	atcclient.SetUniqueFile(flagUniqueFile)
	atcclient.SetIsServing(true)

	roxyresolver.SetLogger(log.Logger.With().Str("package", "roxyresolver").Logger())

	var atc mainutil.ATCClientConfig
	err := atc.Parse(flagATC)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "atc").
			Str("input", flagATC).
			Err(err).
			Msg("--atc: failed to process")
	}

	var web mainutil.GRPCClientConfig
	err = web.Parse(flagWeb)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "web").
			Str("input", flagWeb).
			Err(err).
			Msg("--web: failed to process")
	}

	atcClient, err := atc.NewClient(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "atc").
			Interface("config", atc).
			Err(err).
			Msg("--atc: failed to connect to ATC")
	}

	if atcClient != nil {
		defer func() {
			err := atcClient.Close()
			if err != nil {
				log.Logger.Error().
					Str("subsystem", "atc").
					Err(err).
					Msg("ATCClient: failed to Close")
			}
		}()

		ctx = roxyresolver.WithATCClient(ctx, atcClient)
	}

	interceptor := atcclient.InterceptorFactory{
		DefaultCostPerQuery:   1,
		DefaultCostPerRequest: 1,
	}

	dialOpts := interceptor.DialOptions(
		grpc.WithBlock())

	cc, err := web.Dial(ctx, dialOpts...)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "web").
			Interface("config", web).
			Err(err).
			Msg("--web: failed to connect to Web server")
	}

	if cc != nil {
		defer func() {
			err := cc.Close()
			if err != nil {
				log.Logger.Error().
					Str("subsystem", "web").
					Err(err).
					Msg("grpc.ClientConn: failed to Close")
			}
		}()

		webClient := roxy_v0.NewWebClient(cc)
		for i := 0; i < flagIterations; i++ {
			if i != 0 {
				time.Sleep(flagDelay)
			}

			req := &roxy_v0.WebMessage{}
			req.Headers = make([]*roxy_v0.KeyValue, 4)
			req.Headers[0] = &roxy_v0.KeyValue{Key: ":scheme", Value: "https"}
			req.Headers[1] = &roxy_v0.KeyValue{Key: ":method", Value: "HEAD"}
			req.Headers[2] = &roxy_v0.KeyValue{Key: ":authority", Value: "demo.localhost"}
			req.Headers[3] = &roxy_v0.KeyValue{Key: ":path", Value: "/"}

			log.Logger.Info().
				Int("count", i).
				Interface("req", req).
				Msg("Send")

			wsc, err := webClient.Serve(ctx)
			if err != nil {
				log.Logger.Error().
					Str("subsystem", "web").
					Err(err).
					Msg("WebClient.Serve failed")
				continue
			}

			err = wsc.Send(req)
			if err != nil {
				log.Logger.Error().
					Str("subsystem", "web").
					Err(err).
					Msg("Web_ServeClient.Send failed")
				continue
			}

			err = wsc.CloseSend()
			if err != nil {
				log.Logger.Error().
					Str("subsystem", "web").
					Err(err).
					Msg("Web_ServeClient.CloseSend failed")
				continue
			}

			for {
				resp, err := wsc.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Logger.Error().
						Str("subsystem", "web").
						Err(err).
						Msg("Web_ServeClient.Recv failed")
					break
				}

				log.Logger.Info().
					Int("count", i).
					Interface("resp", resp).
					Msg("Recv")
			}
		}
	}
}
