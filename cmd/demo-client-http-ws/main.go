// Command "demo-client-http-ws" is a demonstration of the "wsutil" library.
//
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/internal/wsutil"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
)

var (
	flagURL        string        = ""
	flagTLS        string        = ""
	flagDelay      time.Duration = 1 * time.Second
	flagIterations int           = 5
)

func init() {
	getopt.SetParameters("")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagURL, "url", 'U', "ws/wss URL to connect to")
	getopt.FlagLong(&flagTLS, "tls", 'T', "TLS configuration")
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

	u, err := url.Parse(flagURL)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagURL).
			Err(err).
			Msg("--url: failed to parse")
	}

	var tlscfg mainutil.TLSClientConfig
	err = tlscfg.Parse(flagTLS)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagTLS).
			Err(err).
			Msg("--tls: failed to parse")
	}

	tlsConfig, err := tlscfg.MakeTLS(u.Hostname())
	if err != nil {
		log.Logger.Fatal().
			Err(err).
			Msg("--tls: failed to instantiate *tls.Config")
	}

	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = tlsConfig

	for i := 0; i < flagIterations; i++ {
		if i != 0 {
			time.Sleep(flagDelay)
		}

		h := make(http.Header, 4)
		h.Set("Origin", "https://localhost")

		conn, _, err := dialer.DialContext(ctx, flagURL, h)
		if err != nil {
			log.Logger.Error().
				Err(err).
				Msg("DialContext")
			continue
		}

		adaptor := wsutil.MakeAdaptor(conn)
		looper := wsutil.NewLooper(
			ctx,
			adaptor,
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
