package main

import (
	"context"
	"os"
	"time"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/roxypb"
)

var (
	flagLogJSON     bool
	flagAdminTarget string = "unix:/var/opt/roxy/lib/admin.socket"
)

func init() {
	getopt.SetParameters("")

	getopt.FlagLong(&flagLogJSON, "log-json", 'J', "if true, log in JSON format")
	getopt.FlagLong(&flagAdminTarget, "server", 's', "address for Admin gRPC interface")
}

func main() {
	getopt.Parse()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.DurationFieldUnit = time.Second
	zerolog.DurationFieldInteger = false
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if !flagLogJSON {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	var cmd string
	if getopt.NArgs() == 0 {
		cmd = "ping"
	} else if getopt.NArgs() == 1 {
		cmd = getopt.Arg(0)
	} else {
		log.Logger.Fatal().
			Int("expect", 1).
			Int("actual", getopt.NArgs()).
			Msg("wrong number of arguments")
	}

	ctx := context.Background()

	dialOpts := make([]grpc.DialOption, 1)
	dialOpts[0] = grpc.WithInsecure()

	cc, err := grpc.DialContext(ctx, flagAdminTarget, dialOpts...)
	if err != nil {
		log.Logger.Fatal().
			Err(err).
			Msg("failed to Dial")
	}
	defer cc.Close()

	admin := roxypb.NewAdminClient(cc)
	switch cmd {
	case "ping":
		_, err := admin.Ping(ctx, &roxypb.PingRequest{})
		if err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("call to /roxy.Admin/Ping failed")
		}

	case "reload":
		_, err := admin.Reload(ctx, &roxypb.ReloadRequest{})
		if err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("call to /roxy.Admin/Reload failed")
		}

	case "shutdown":
		_, err := admin.Shutdown(ctx, &roxypb.ShutdownRequest{})
		if err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("call to /roxy.Admin/Shutdown failed")
		}

	default:
		log.Logger.Fatal().
			Str("cmd", cmd).
			Msg("unknown command")
	}

	log.Logger.Info().
		Msg("OK")
}
