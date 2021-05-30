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

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/membership"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
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

var gMultiServer mainutil.MultiServer

func main() {
	getopt.Parse()

	mainutil.InitVersion()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	ctx = announcer.WithDeclaredCPS(ctx, 64.0)

	mainutil.SetUniqueFile(flagUniqueFile)

	roxyresolver.SetLogger(log.Logger.With().Str("package", "roxyresolver").Logger())

	var httpListenConfig mainutil.ListenConfig
	var grpcListenConfig mainutil.ListenConfig
	var grpcServerOpts []grpc.ServerOption
	var zac mainutil.ZKAnnounceConfig
	var eac mainutil.EtcdAnnounceConfig
	var aac mainutil.ATCAnnounceConfig
	processFlags(
		&httpListenConfig,
		&grpcListenConfig,
		&grpcServerOpts,
		&zac,
		&eac,
		&aac,
	)

	var corpus []byte
	if flagShakespeareFile == "" {
		corpus = []byte("Hello, world!\r\n")
	} else {
		var err error
		corpus, err = ioutil.ReadFile(flagShakespeareFile)
		if err != nil {
			log.Logger.Fatal().
				Str("path", flagShakespeareFile).
				Err(err).
				Msg("--shakespeare-file: failed to read file")
		}
	}

	ann := new(announcer.Announcer)

	zkConn, err := zac.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "zk").
			Interface("config", zac).
			Err(err).
			Msg("--zk: failed to connect to ZooKeeper")
	}

	if zkConn != nil {
		gMultiServer.OnExit(func(context.Context) error {
			zkConn.Close()
			return nil
		})

		ctx = roxyresolver.WithZKConn(ctx, zkConn)

		if err := zac.AddTo(zkConn, ann); err != nil {
			log.Logger.Fatal().
				Str("subsystem", "zk").
				Interface("config", zac).
				Err(err).
				Msg("--announce-zk: failed to announce to ZooKeeper")
		}
	}

	etcd, err := eac.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "etcd").
			Interface("config", eac).
			Err(err).
			Msg("--announce-etcd: failed to connect to etcd")
	}

	if etcd != nil {
		gMultiServer.OnExit(func(context.Context) error {
			return etcd.Close()
		})

		ctx = roxyresolver.WithEtcdV3Client(ctx, etcd)

		if err := eac.AddTo(etcd, ann); err != nil {
			log.Logger.Fatal().
				Str("subsystem", "etcd").
				Interface("config", eac).
				Err(err).
				Msg("--announce-etcd: failed to announce to etcd")
		}
	}

	atcClient, err := aac.NewClient(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "atc").
			Interface("config", aac).
			Err(err).
			Msg("--announce-atc: failed to connect to ATC")
	}

	if atcClient != nil {
		gMultiServer.OnExit(func(context.Context) error {
			return atcClient.Close()
		})

		ctx = roxyresolver.WithATCClient(ctx, atcClient)

		if err := aac.AddTo(atcClient, ann); err != nil {
			log.Logger.Fatal().
				Str("subsystem", "atc").
				Interface("config", aac).
				Err(err).
				Msg("--announce-atc: failed to announce to ATC")
		}
	}

	interceptor := atcclient.InterceptorFactory{
		DefaultCostPerQuery:   1,
		DefaultCostPerRequest: 1,
	}

	var httpHandler http.Handler
	httpHandler = demoHandler{corpus: corpus}
	httpHandler = interceptor.Handler(httpHandler)
	httpServer := &http.Server{
		Handler: httpHandler,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	grpcServerOpts = interceptor.ServerOptions(grpcServerOpts...)
	grpcServer := grpc.NewServer(grpcServerOpts...)
	grpc_health_v1.RegisterHealthServer(grpcServer, gMultiServer.HealthServer())
	roxy_v0.RegisterWebServer(grpcServer, &webServerServer{corpus: corpus})

	httpListener, err := httpListenConfig.Listen(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemHTTP).
			Interface("config", httpListenConfig).
			Err(err).
			Msg("failed to Listen")
	}

	grpcListener, err := grpcListenConfig.ListenNoTLS(ctx)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Interface("config", grpcListenConfig).
			Err(err).
			Msg("failed to Listen")
	}

	var r membership.Roxy
	r.Ready = true
	r.AdditionalPorts = make(map[string]uint16, 2)
	if tcpAddr, ok := httpListener.Addr().(*net.TCPAddr); ok {
		r.IP = tcpAddr.IP
		r.Zone = tcpAddr.Zone
		r.PrimaryPort = uint16(tcpAddr.Port)
		r.AdditionalPorts[constants.SubsystemHTTP] = uint16(tcpAddr.Port)
	}
	if tcpAddr, ok := grpcListener.Addr().(*net.TCPAddr); ok {
		r.IP = tcpAddr.IP
		r.Zone = tcpAddr.Zone
		r.PrimaryPort = uint16(tcpAddr.Port)
		r.AdditionalPorts[constants.SubsystemGRPC] = uint16(tcpAddr.Port)
	}

	gMultiServer.AddHTTPServer(constants.SubsystemHTTP, httpServer, httpListener)
	gMultiServer.AddGRPCServer(constants.SubsystemGRPC, grpcServer, grpcListener)
	gMultiServer.AddAnnouncer(ann, &r)

	gMultiServer.OnReload(mainutil.RotateLogs)

	gMultiServer.SetHealth("", true)
	gMultiServer.WatchHealth(func(subsystemName string, isHealthy bool, isStopped bool) {
		if subsystemName == "" {
			atcclient.SetIsServing(isHealthy)
		}
	})

	err = gMultiServer.Run(ctx)
	if err != nil {
		log.Logger.Error().
			Err(err).
			Msg("Run")
		return
	}
}

func processFlags(
	httpListenConfig *mainutil.ListenConfig,
	grpcListenConfig *mainutil.ListenConfig,
	grpcServerOpts *[]grpc.ServerOption,
	zac *mainutil.ZKAnnounceConfig,
	eac *mainutil.EtcdAnnounceConfig,
	aac *mainutil.ATCAnnounceConfig,
) {
	expanded, err := roxyutil.ExpandPath(flagShakespeareFile)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagShakespeareFile).
			Err(err).
			Msg("--shakespeare-file: failed to process path")
	}
	flagShakespeareFile = expanded

	err = httpListenConfig.Parse(flagListenHTTP)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemHTTP).
			Str("input", flagListenHTTP).
			Err(err).
			Msg("--listen-http: failed to parse config")
	}

	log.Logger.Trace().
		Str("subsystem", constants.SubsystemHTTP).
		Interface("config", httpListenConfig).
		Msg("ready")

	err = grpcListenConfig.Parse(flagListenGRPC)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Str("input", flagListenGRPC).
			Err(err).
			Msg("--listen-grpc: failed to parse config")
	}

	*grpcServerOpts, err = grpcListenConfig.TLS.MakeGRPCServerOptions()
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", constants.SubsystemGRPC).
			Interface("config", grpcListenConfig.TLS).
			Err(err).
			Msg("--listen-grpc: failed to MakeGRPCServerOptions")
	}

	log.Logger.Trace().
		Str("subsystem", constants.SubsystemGRPC).
		Interface("config", grpcListenConfig).
		Msg("ready")

	err = zac.Parse(flagAnnounceZK)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "zk").
			Str("input", flagAnnounceZK).
			Err(err).
			Msg("--announce-zk: failed to parse config")
	}

	log.Logger.Trace().
		Str("subsystem", "zk").
		Interface("config", zac).
		Msg("ready")

	err = eac.Parse(flagAnnounceEtcd)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "etcd").
			Str("input", flagAnnounceEtcd).
			Err(err).
			Msg("--announce-etcd: failed to parse config")
	}

	log.Logger.Trace().
		Str("subsystem", "etcd").
		Interface("config", eac).
		Msg("ready")

	err = aac.Parse(flagAnnounceATC)
	if err != nil {
		log.Logger.Fatal().
			Str("subsystem", "atc").
			Str("input", flagAnnounceATC).
			Err(err).
			Msg("--announce-atc: failed to parse config")
	}

	log.Logger.Trace().
		Str("subsystem", "atc").
		Interface("config", aac).
		Msg("ready")
}

type demoHandler struct {
	corpus []byte
}

func (h demoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Logger.Debug().
		Str("httpHost", r.Host).
		Str("httpMethod", r.Method).
		Str("httpPath", r.URL.Path).
		Msg("HTTP")

	atcclient.Spend(1)

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
	hdrs.Set(constants.HeaderContentType, constants.ContentTypeTextPlain)
	hdrs.Set(constants.HeaderContentLen, strconv.Itoa(len(h.corpus)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.corpus)
}

var _ http.Handler = demoHandler{}

type webServerServer struct {
	roxy_v0.UnimplementedWebServer
	corpus []byte
}

func (s *webServerServer) Serve(ws roxy_v0.Web_ServeServer) (err error) {
	log.Logger.Debug().
		Str("rpcService", "roxy.v0.Web").
		Str("rpcMethod", "Serve").
		Str("rpcInterface", "primary").
		Msg("RPC")

	hIn := make([]*roxy_v0.KeyValue, 0, 32)
	bIn := []byte(nil)
	schemeIn := ""
	methodIn := ""
	hostIn := ""
	pathIn := ""

	for {
		var msg *roxy_v0.WebMessage
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
				case constants.PseudoHeaderScheme:
					schemeIn = kv.Value
				case constants.PseudoHeaderMethod:
					methodIn = kv.Value
				case constants.PseudoHeaderAuthority:
					hostIn = kv.Value
				case constants.PseudoHeaderPath:
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
	hOut := make([]*roxy_v0.KeyValue, 1, 32)
	hOut[0] = &roxy_v0.KeyValue{
		Key:   constants.PseudoHeaderStatus,
		Value: constants.Status200,
	}

	defer func() {
		if err == nil {
			hOut = append(hOut, &roxy_v0.KeyValue{
				Key:   constants.HeaderContentLen,
				Value: strconv.Itoa(len(bOut)),
			})
			if methodIn == http.MethodHead {
				bOut = nil
			}
			if len(bOut) < maxBodyChunk {
				err = ws.Send(&roxy_v0.WebMessage{
					Headers:   hOut,
					BodyChunk: bOut,
				})
			} else {
				err = ws.Send(&roxy_v0.WebMessage{Headers: hOut})
				i, j := 0, len(bOut)
				for err == nil && i < j {
					k := i + maxBodyChunk
					if k > j {
						k = j
					}
					err = ws.Send(&roxy_v0.WebMessage{BodyChunk: bOut[i:k]})
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
		hOut = append(hOut, &roxy_v0.KeyValue{
			Key:   constants.HeaderAllow,
			Value: constants.AllowGET,
		})
		return
	}

	if methodIn != http.MethodGet && methodIn != http.MethodPost {
		hOut[0].Value = "405"
		hOut = append(hOut, &roxy_v0.KeyValue{
			Key:   constants.HeaderAllow,
			Value: constants.AllowGET,
		})
		return
	}

	bOut = s.corpus
	hOut = append(hOut, &roxy_v0.KeyValue{
		Key:   constants.HeaderContentType,
		Value: constants.ContentTypeTextPlain,
	})
	return
}

type kvList []*roxy_v0.KeyValue

var specialHeaders = map[string]int{
	constants.PseudoHeaderScheme:    -5,
	constants.PseudoHeaderMethod:    -4,
	constants.PseudoHeaderAuthority: -3,
	constants.PseudoHeaderPath:      -2,
	constants.PseudoHeaderStatus:    -1,
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
