// Command "demo-client-grpc-ws" is a demonstration of the "wsutil" library.
//
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/internal/wsutil"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

var (
	flagWeb        string        = ""
	flagDelay      time.Duration = 1 * time.Second
	flagIterations int           = 5
)

func init() {
	getopt.SetParameters("")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagWeb, "web", 'W', "gRPC roxy.v0.Web server to connect to")
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
	ctx = log.Logger.WithContext(ctx)

	roxyresolver.SetLogger(log.Logger.With().Str("package", "roxyresolver").Logger())

	var web mainutil.GRPCClientConfig
	err := web.Parse(flagWeb)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "web").
			Str("input", flagWeb).
			Err(err).
			Msg("--web: failed to process")
	}

	cc, err := web.Dial(ctx, grpc.WithBlock())
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

			log.Logger.Info().
				Int("count", i).
				Msg("Send")

			wsc, err := webClient.Socket(ctx)
			if err != nil {
				log.Logger.Error().
					Str("subsystem", "web").
					Err(err).
					Msg("WebClient.Socket failed")
				continue
			}

			looper := wsutil.NewLooper(
				ctx,
				wsc,
				wsutil.OnText(func(ctx context.Context, looper *wsutil.Looper, text string) {
					log.Logger.Info().
						Str("text", text).
						Msg("OnText")
				}),
				wsutil.OnBinary(func(ctx context.Context, looper *wsutil.Looper, data []byte) {
					log.Logger.Warn().
						Int("len", len(data)).
						Msg("OnBinary")
				}),
			)

			for j := 0; j < 3; j++ {
				k := rand.Float64()
				if k < 0.5 {
					var x struct {
						A float64 `json:"a"`
						B float64 `json:"b"`
					}
					x.A = rand.Float64() * 1024
					x.B = rand.Float64() * 1024

					var buf bytes.Buffer
					buf.Grow(32)
					e := json.NewEncoder(&buf)
					err := e.Encode(&x)
					if err != nil {
						panic(err)
					}

					log.Logger.Info().
						Float64("A", x.A).
						Float64("B", x.B).
						Msg("Add")

					text := buf.String()
					looper.SendTextMessage(text)
				} else {
					x0 := rand.Uint64()
					x1 := rand.Uint64()
					buf := make([]byte, 16)
					binary.BigEndian.PutUint64(buf[0:], x0)
					binary.BigEndian.PutUint64(buf[8:], x1)

					log.Logger.Info().
						Str("data", hex.EncodeToString(buf)).
						Msg("SHA256")

					looper.SendBinaryMessage(buf)
				}
			}

			looper.SendClose(websocket.CloseNormalClosure, "")
			err = looper.Wait()
			if err != nil {
				log.Logger.Error().
					Err(err).
					Msg("WebSocket communication failed")
			}
		}
	}
}
