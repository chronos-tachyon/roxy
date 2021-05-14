// Command "demo-backend" is a demonstration of a server which uses the Roxy
// libraries "membership", "announce", and "mainutil", and which serves web
// content via both HTTP(S) and gRPC.
//
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/roxypb"
)

const maxBodyChunk = 1 << 20 // 1 MiB

var (
	flagListenHTTP      string = "127.0.0.1:8000"
	flagListenGRPC      string = "127.0.0.1:8001"
	flagAnnounceZK      string = ""
	flagAnnounceEtcd    string = ""
	flagAnnounceATC     string = ""
	flagShakespeareFile string = "/dev/null"
	flagUniqueFile      string = "/var/opt/roxy/lib/state/demo-backend.id"
)

func init() {
	getopt.SetParameters("")

	mainutil.SetAppVersion(mainutil.RoxyVersion())
	mainutil.RegisterVersionFlag()
	mainutil.RegisterLoggingFlags()

	getopt.FlagLong(&flagListenHTTP, "listen-http", 'H', "ip:port to listen on (HTTP or HTTPS)")
	getopt.FlagLong(&flagListenGRPC, "listen-grpc", 'G', "ip:port to listen on (gRPC or gRPCS)")
	getopt.FlagLong(&flagAnnounceZK, "announce-zk", 'Z', "ZooKeeper announce configuration")
	getopt.FlagLong(&flagAnnounceEtcd, "announce-etcd", 'E', "etcd announce configuration")
	getopt.FlagLong(&flagAnnounceATC, "announce-atc", 'A', "ATC announce configuration")
	getopt.FlagLong(&flagShakespeareFile, "shakespeare-file", 'f', "file to serve; recommend using https://ocw.mit.edu/ans7870/6/6.006/s08/lecturenotes/files/t8.shakespeare.txt")
	getopt.FlagLong(&flagUniqueFile, "unique-file", 'U', "file containing a unique ID for the ATC announcer")
}

var (
	gMultiServer  mainutil.MultiServer
	gHealthServer mainutil.HealthServer
)

func main() {
	getopt.Parse()

	mainutil.InitVersion()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	mainutil.SetUniqueFile(flagUniqueFile)

	roxyresolver.SetLogger(log.Logger.With().Str("package", "roxyresolver").Logger())

	expanded, err := roxyutil.ExpandPath(flagShakespeareFile)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagShakespeareFile).
			Err(err).
			Msg("--shakespeare-file: failed to process path")
	}
	flagShakespeareFile = expanded

	var corpus []byte
	if flagShakespeareFile == "" {
		corpus = []byte("Hello, world!\r\n")
	} else {
		corpus, err = ioutil.ReadFile(flagShakespeareFile)
		if err != nil {
			log.Logger.Fatal().
				Str("path", flagShakespeareFile).
				Err(err).
				Msg("--shakespeare-file: failed to read file")
		}
	}

	var httpListenConfig mainutil.ListenConfig
	err = httpListenConfig.Parse(flagListenHTTP)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagListenHTTP).
			Err(err).
			Msg("--listen-http: failed to parse config")
	}

	var grpcListenConfig mainutil.ListenConfig
	err = grpcListenConfig.Parse(flagListenGRPC)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagListenGRPC).
			Err(err).
			Msg("--listen-grpc: failed to parse config")
	}

	var zac mainutil.ZKAnnounceConfig
	err = zac.Parse(flagAnnounceZK)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagAnnounceZK).
			Err(err).
			Msg("--announce-zk: failed to parse")
	}

	var eac mainutil.EtcdAnnounceConfig
	err = eac.Parse(flagAnnounceEtcd)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagAnnounceEtcd).
			Err(err).
			Msg("--announce-etcd: failed to parse etcd config")
	}

	var aac mainutil.ATCAnnounceConfig
	err = aac.Parse(flagAnnounceATC)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagAnnounceATC).
			Err(err).
			Msg("--announce-atc: failed to parse ATC config")
	}

	ann := new(announcer.Announcer)
	gHealthServer.Set("", true)

	gMultiServer.OnShutdown(func(alreadyTermed bool) error {
		gHealthServer.Stop()
		return nil
	})

	zkconn, err := zac.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Interface("config", zac).
			Err(err).
			Msg("--zk: failed to connect to ZooKeeper")
	}

	if zkconn != nil {
		defer zkconn.Close()

		ctx = roxyresolver.WithZKConn(ctx, zkconn)

		if err := zac.AddTo(zkconn, ann); err != nil {
			log.Logger.Fatal().
				Interface("config", zac).
				Err(err).
				Msg("--announce-zk: failed to announce to ZooKeeper")
		}
	}

	etcd, err := eac.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Interface("config", eac).
			Err(err).
			Msg("--announce-etcd: failed to connect to etcd")
	}

	if etcd != nil {
		defer func() {
			_ = etcd.Close()
		}()

		ctx = roxyresolver.WithEtcdV3Client(ctx, etcd)

		if err := eac.AddTo(etcd, ann); err != nil {
			log.Logger.Fatal().
				Interface("config", eac).
				Err(err).
				Msg("--announce-etcd: failed to announce to etcd")
		}
	}

	atcClient, err := aac.NewClient(ctx)
	if err != nil {
		log.Logger.Fatal().
			Interface("config", aac).
			Err(err).
			Msg("--announce-atc: failed to connect to ATC")
	}

	if atcClient != nil {
		defer func() {
			_ = atcClient.Close()
		}()

		ctx = roxyresolver.WithATCClient(ctx, atcClient)

		if err := aac.AddTo(atcClient, nil, ann); err != nil {
			log.Logger.Fatal().
				Interface("config", aac).
				Err(err).
				Msg("--announce-atc: failed to announce to ATC")
		}
	}

	httpServer := &http.Server{
		Handler: demoHandler{corpus: corpus},
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	httpListener, err := httpListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "http").
			Interface("config", httpListenConfig).
			Err(err).
			Msg("failed to Listen")
	}
	gMultiServer.AddHTTPServer("http", httpServer, httpListener)

	grpcServer := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, &gHealthServer)
	roxypb.RegisterWebServerServer(grpcServer, &webServerServer{corpus: corpus})

	grpcListener, err := grpcListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("server", "grpc").
			Interface("config", grpcListenConfig).
			Err(err).
			Msg("failed to Listen")
	}
	gMultiServer.AddGRPCServer("grpc", grpcServer, grpcListener)

	gMultiServer.OnRun(func() {
		var r membership.Roxy
		r.Ready = true
		r.AdditionalPorts = make(map[string]uint16, 2)
		if tcpAddr, ok := httpListener.Addr().(*net.TCPAddr); ok {
			r.IP = tcpAddr.IP
			r.Zone = tcpAddr.Zone
			r.PrimaryPort = uint16(tcpAddr.Port)
			r.AdditionalPorts["http"] = uint16(tcpAddr.Port)
		}
		if tcpAddr, ok := grpcListener.Addr().(*net.TCPAddr); ok {
			r.IP = tcpAddr.IP
			r.Zone = tcpAddr.Zone
			r.PrimaryPort = uint16(tcpAddr.Port)
			r.AdditionalPorts["grpc"] = uint16(tcpAddr.Port)
		}
		if err := ann.Announce(ctx, &r); err != nil {
			log.Logger.Fatal().
				Err(err).
				Msg("failed to Announce")
		}
	})

	gMultiServer.OnShutdown(func(alreadyTermed bool) error {
		var action string
		var err error
		if alreadyTermed {
			action = "Close"
			err = ann.Close()
		} else {
			action = "Withdraw"
			err = ann.Withdraw(ctx)
		}
		if err != nil {
			log.Logger.Error().
				Str("action", action).
				Err(err).
				Msg("failed to withdraw one or more announcements")
			return err
		}
		return nil
	})

	gMultiServer.OnReload(mainutil.RotateLogs)

	gMultiServer.OnRun(func() {
		log.Logger.Info().
			Msg("Running")
	})

	gMultiServer.Run()

	log.Logger.Info().
		Msg("Exit")
}

type demoHandler struct {
	corpus []byte
}

func (h demoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headerNames := make([]string, 0, len(r.Header))
	for key := range r.Header {
		headerNames = append(headerNames, strings.ToLower(key))
	}
	sort.Strings(headerNames)

	size, _ := io.Copy(io.Discard, r.Body)

	var buf bytes.Buffer
	buf.Grow(256)
	fmt.Fprintf(&buf, "HTTP - [%s] [%s] [%s] [%s]\n", r.Method, r.Host, r.URL, r.Proto)
	for _, name := range headerNames {
		values := r.Header.Values(name)
		for _, value := range values {
			fmt.Fprintf(&buf, "[%s]: %s\n", name, value)
		}
	}
	fmt.Fprintf(&buf, "body: %d bytes\n", size)
	fmt.Fprintf(&buf, "\n")
	_, _ = os.Stdout.Write(buf.Bytes())

	hdrs := w.Header()
	hdrs.Set("content-type", "text/plain; charset=utf-8")
	hdrs.Set("content-length", strconv.Itoa(len(h.corpus)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.corpus)
}

var _ http.Handler = demoHandler{}

type webServerServer struct {
	roxypb.UnimplementedWebServerServer
	corpus []byte
}

func (s *webServerServer) Serve(ws roxypb.WebServer_ServeServer) (err error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.WebServer").
		Str("rpcMethod", "Serve").
		Str("rpcInterface", "primary").
		Msg("RPC")

	hIn := make([]*roxypb.KeyValue, 0, 32)
	bIn := []byte(nil)
	schemeIn := ""
	methodIn := ""
	hostIn := ""
	pathIn := ""

	for {
		var msg *roxypb.WebMessage
		msg, err = ws.Recv()
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return err
		}
		if len(msg.BodyChunk) != 0 {
			bIn = append(bIn, msg.BodyChunk...)
		}
		if len(msg.Headers) != 0 {
			for _, kv := range msg.Headers {
				switch kv.Key {
				case ":scheme":
					schemeIn = kv.Value
				case ":method":
					methodIn = kv.Value
				case ":authority":
					hostIn = kv.Value
				case ":path":
					pathIn = kv.Value
				default:
					hIn = append(hIn, kv)
				}
			}
		}
		if len(msg.Trailers) != 0 {
			hIn = append(hIn, msg.Trailers...)
		}
	}

	var buf bytes.Buffer
	buf.Grow(256)
	fmt.Fprintf(&buf, "GRPC - [%s] [%s] [%s] [%s]\n", schemeIn, methodIn, hostIn, pathIn)
	kvList(hIn).Sort()
	for _, kv := range hIn {
		fmt.Fprintf(&buf, "[%s]: %s\n", kv.Key, kv.Value)
	}
	fmt.Fprintf(&buf, "body: %d bytes\n", len(bIn))
	fmt.Fprintf(&buf, "\n")
	_, _ = os.Stdout.Write(buf.Bytes())

	bOut := []byte(nil)
	hOut := make([]*roxypb.KeyValue, 1, 32)
	hOut[0] = &roxypb.KeyValue{Key: ":status", Value: "200"}

	defer func() {
		if err == nil {
			hOut = append(hOut, &roxypb.KeyValue{Key: "content-length", Value: strconv.Itoa(len(bOut))})
			if methodIn == http.MethodHead {
				bOut = nil
			}
			if len(bOut) < maxBodyChunk {
				err = ws.Send(&roxypb.WebMessage{
					Headers:   hOut,
					BodyChunk: bOut,
				})
			} else {
				err = ws.Send(&roxypb.WebMessage{Headers: hOut})
				i, j := 0, len(bOut)
				for err == nil && i < j {
					k := i + maxBodyChunk
					if k > j {
						k = j
					}
					err = ws.Send(&roxypb.WebMessage{BodyChunk: bOut[i:k]})
					i = k
				}
			}
		}
	}()

	u, e := url.Parse(pathIn)
	if e != nil {
		hOut[0].Value = "400"
		return
	}
	u.Path = path.Clean(u.Path)

	if u.Path != "/" {
		hOut[0].Value = "404"
		return
	}

	if methodIn == http.MethodOptions {
		hOut[0].Value = "204"
		hOut = append(hOut, &roxypb.KeyValue{Key: "allow", Value: "OPTIONS, GET, HEAD"})
		return
	}

	if methodIn != http.MethodGet && methodIn != http.MethodPost {
		hOut[0].Value = "405"
		hOut = append(hOut, &roxypb.KeyValue{Key: "allow", Value: "OPTIONS, GET, HEAD"})
		return
	}

	bOut = s.corpus
	hOut = append(hOut, &roxypb.KeyValue{Key: "content-type", Value: "text/plain; charset=utf-8"})
	return
}

type kvList []*roxypb.KeyValue

var specialHeaders = map[string]int{
	":scheme":    -4,
	":method":    -3,
	":authority": -2,
	":path":      -1,
}

func (list kvList) Len() int {
	return len(list)
}

func (list kvList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list kvList) Less(i, j int) bool {
	a, b := list[i].Key, list[j].Key
	x, y := specialHeaders[a], specialHeaders[b]
	if x != y {
		return x < y
	}
	return a < b
}

func (list kvList) Sort() {
	sort.Stable(list)
}
