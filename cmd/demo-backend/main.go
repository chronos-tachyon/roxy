package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-zookeeper/zk"
	getopt "github.com/pborman/getopt/v2"
	v3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/chronos-tachyon/roxy/internal/balancedclient"
	"github.com/chronos-tachyon/roxy/roxypb"
)

const maxBodyChunk = 1 << 20 // 1 MiB

var (
	flagListenHTTP      string = "127.0.0.1:8000"
	flagListenGRPC      string = "127.0.0.1:8001"
	flagCertFile        string = ""
	flagKeyFile         string = ""
	flagZKServers       string = "127.0.0.1:2181"
	flagZKPath          string = "/demo"
	flagEtcdServers     string = "http://127.0.0.1:2379"
	flagEtcdUsername    string = ""
	flagEtcdPassword    string = ""
	flagEtcdHTTPPath    string = "/demo/http"
	flagEtcdGRPCPath    string = "/demo/grpc"
	flagATCServers      string = "ipv4:127.0.0.1:2987"
	flagATCServerName   string = ""
	flagATCName         string = "demo"
	flagUnique          string = ""
	flagShakespeareFile string = "/dev/null"
)

func init() {
	getopt.SetParameters("")

	getopt.FlagLong(&flagListenHTTP, "listen-http", 'l', "ip:port to listen on (HTTP or HTTPS)")
	getopt.FlagLong(&flagListenGRPC, "listen-grpc", 'L', "ip:port to listen on (gRPC or gRPCS)")
	getopt.FlagLong(&flagCertFile, "cert-file", 'c', "path to PEM-format cert file (enables TLS)")
	getopt.FlagLong(&flagKeyFile, "key-file", 'k', "path to PEM-format key file (defaults to --cert-file)")
	getopt.FlagLong(&flagZKServers, "zk-servers", 'Z', "comma-separated list of ZooKeeper servers to advertise to")
	getopt.FlagLong(&flagZKPath, "zk-path", 'z', "ZooKeeper path to advertise as")
	getopt.FlagLong(&flagEtcdServers, "etcd-servers", 'E', "comma-separated list of Etcd endpoints to advertise to")
	getopt.FlagLong(&flagEtcdUsername, "etcd-username", 'u', "username for Etcd")
	getopt.FlagLong(&flagEtcdPassword, "etcd-password", 'p', "password for Etcd")
	getopt.FlagLong(&flagEtcdHTTPPath, "etcd-http-path", 0, "Etcd key to advertise as (HTTP or HTTPS)")
	getopt.FlagLong(&flagEtcdGRPCPath, "etcd-grpc-path", 0, "Etcd key to advertise as (gRPC or gRPCS)")
	getopt.FlagLong(&flagATCServers, "atc-servers", 'A', "gRPC target of ATC servers to advertise to")
	getopt.FlagLong(&flagATCServerName, "atc-servername", 0, "TLS ServerName of ATC servers (enables TLS)")
	getopt.FlagLong(&flagATCName, "atc-name", 'a', "ATC service name to advertise as")
	getopt.FlagLong(&flagUnique, "unique", 'H', "unique identifier; defaults to os.Hostname()")
	getopt.FlagLong(&flagShakespeareFile, "shakespeare-file", 'f', "file to serve; recommend using https://ocw.mit.edu/ans7870/6/6.006/s08/lecturenotes/files/t8.shakespeare.txt")
}

func main() {
	getopt.Parse()

	ctx := context.Background()
	ctx, cancelfn := context.WithCancel(ctx)
	defer cancelfn()

	var err error
	if flagUnique == "" {
		flagUnique, err = os.Hostname()
		if err != nil {
			panic(fmt.Errorf("failed to retrieve hostname: %w", err))
		}
		flagUnique = strings.TrimRight(flagUnique, ".")
	}
	if !regexp.MustCompile(`^[0-9A-Za-z]+(?:[._-][0-9A-Za-z]+)*$`).MatchString(flagUnique) {
		panic(fmt.Errorf("--unique: invalid unique string %q", flagUnique))
	}

	var corpus []byte
	if flagShakespeareFile == "" {
		corpus = []byte("Hello, world!\r\n")
	} else {
		corpus, err = ioutil.ReadFile(flagShakespeareFile)
		if err != nil {
			panic(fmt.Errorf("--shakespeare-file: failed to read file %q: %w", flagShakespeareFile, err))
		}
	}

	var (
		httpAddr *net.TCPAddr
		grpcAddr *net.TCPAddr
	)

	if flagListenHTTP != "" {
		httpAddr, err = parseHostPort("--listen-http", flagListenHTTP)
		if err != nil {
			panic(err)
		}
	}

	if flagListenGRPC != "" {
		grpcAddr, err = parseHostPort("--listen-grpc", flagListenGRPC)
		if err != nil {
			panic(err)
		}
	}

	var serverTLSConfig *tls.Config
	if flagKeyFile == "" {
		flagKeyFile = flagCertFile
	}
	if flagCertFile != "" {
		cert, err := tls.LoadX509KeyPair(flagCertFile, flagKeyFile)
		if err != nil {
			panic(fmt.Errorf("--cert-file/--key-file: failed to load: %w", err))
		}
		serverTLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2", "http/1.1"},
		}
	}

	var wg sync.WaitGroup

	httpListenAndServe := func() error { return nil }
	httpShutdown := func(_ context.Context) error { return nil }
	if httpAddr != nil {
		s := &http.Server{
			Addr:      httpAddr.String(),
			TLSConfig: serverTLSConfig,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headerNames := make([]string, 0, len(r.Header))
				for key := range r.Header {
					headerNames = append(headerNames, strings.ToLower(key))
				}

				sort.Strings(headerNames)

				fmt.Printf("[%s] [%s] [%s] [%s]\n", r.Method, r.Host, r.URL, r.Proto)
				for _, name := range headerNames {
					values := r.Header.Values(name)
					for _, value := range values {
						fmt.Printf("[%s]: %s\n", name, value)
					}
				}
				fmt.Printf("\n")

				hdrs := w.Header()
				hdrs.Set("content-type", "text/plain; charset=utf-8")
				hdrs.Set("content-length", strconv.Itoa(len(corpus)))
				w.WriteHeader(http.StatusOK)
				w.Write(corpus)
			}),
			BaseContext: func(_ net.Listener) context.Context {
				return ctx
			},
		}

		httpListenAndServe = s.ListenAndServe
		httpShutdown = s.Shutdown

		httpListenAndServe = func() error {
			var l net.Listener
			var err error
			l, err = net.ListenTCP("tcp", httpAddr)
			if err != nil {
				return fmt.Errorf("--listen-http: failed to Listen on address %v: %w", httpAddr, err)
			}
			if serverTLSConfig != nil {
				l = tls.NewListener(l, s.TLSConfig)
			}
			return s.Serve(l)
		}
	}

	grpcListenAndServe := func() error { return nil }
	grpcShutdown := func(_ context.Context) error { return nil }
	if grpcAddr != nil {
		s := grpc.NewServer()
		wss := &webServerServer{corpus: corpus}
		roxypb.RegisterWebServerServer(s, wss)

		grpcListenAndServe = func() error {
			var l net.Listener
			var err error
			l, err = net.ListenTCP("tcp", grpcAddr)
			if err != nil {
				return fmt.Errorf("--listen-grpc: failed to Listen on %v: %w", grpcAddr, err)
			}
			if serverTLSConfig != nil {
				l = tls.NewListener(l, serverTLSConfig)
			}
			return s.Serve(l)
		}

		grpcShutdown = func(_ context.Context) error {
			s.Stop()
			return nil
		}
	}

	advertiseZK := func() error { return nil }
	shutdownZK := func() error { return nil }
	if flagZKServers != "" {
		if flagZKPath == "" || flagZKPath[0] != '/' || flagZKPath[len(flagZKPath)-1] == '/' {
			panic(fmt.Errorf("--zk-path: invalid path %q", flagZKPath))
		}

		servers := strings.Split(flagZKServers, ",")
		zkconn, _, err := zk.Connect(servers, 30*time.Second)
		if err != nil {
			panic(err)
		}

		advertiseZK = func() error {
			if httpAddr == nil && grpcAddr == nil {
				return nil
			}

			var member balancedclient.ServerSetMember
			if httpAddr != nil {
				member.ServiceEndpoint = balancedclient.ServerSetEndpoint{
					Host: hostAndZone(httpAddr),
					Port: uint16(httpAddr.Port),
				}
			} else if grpcAddr != nil {
				member.ServiceEndpoint = balancedclient.ServerSetEndpoint{
					Host: hostAndZone(grpcAddr),
					Port: uint16(grpcAddr.Port),
				}
			}
			member.AdditionalEndpoints = make(map[string]balancedclient.ServerSetEndpoint, 2)
			if httpAddr != nil {
				member.AdditionalEndpoints["http"] = balancedclient.ServerSetEndpoint{
					Host: hostAndZone(httpAddr),
					Port: uint16(httpAddr.Port),
				}
			}
			if grpcAddr != nil {
				member.AdditionalEndpoints["grpc"] = balancedclient.ServerSetEndpoint{
					Host: hostAndZone(grpcAddr),
					Port: uint16(grpcAddr.Port),
				}
			}
			member.Status = balancedclient.StatusAlive

			memberData, err := json.Marshal(&member)
			if err != nil {
				return err
			}

			memberPath := path.Join(flagZKPath, flagUnique)
			_, err = zkconn.CreateProtectedEphemeralSequential(memberPath, memberData, zk.WorldACL(zk.PermAll))
			if err != nil {
				return err
			}

			return nil
		}

		shutdownZK = func() error {
			zkconn.Close()
			return nil
		}
	}

	advertiseEtcd := func() error { return nil }
	shutdownEtcd := func() error { return nil }
	if flagEtcdServers != "" {
		endpoints := strings.Split(flagEtcdServers, ",")

		etcd, err := v3.New(v3.Config{
			Endpoints:        endpoints,
			AutoSyncInterval: 1 * time.Minute,
			DialTimeout:      5 * time.Second,
			Username:         flagEtcdUsername,
			Password:         flagEtcdPassword,
		})
		if err != nil {
			panic(err)
		}

		lease, err := etcd.Lease.Grant(ctx, 30)
		if err != nil {
			panic(err)
		}

		ch, err := etcd.Lease.KeepAlive(ctx, lease.ID)
		if err != nil {
			panic(err)
		}

		wg.Add(1)
		go func() {
			for range ch {
			}
			wg.Done()
		}()

		advertiseEtcd = func() error {
			if httpAddr != nil && flagEtcdHTTPPath != "" {
				var member balancedclient.GRPCMember
				member.Op = balancedclient.GRPCOpAdd
				member.Addr = httpAddr.String()
				memberData, err := json.Marshal(&member)
				if err != nil {
					return err
				}

				httpPath := flagEtcdHTTPPath + "/" + flagUnique
				_, err = etcd.KV.Put(ctx, httpPath, string(memberData), v3.WithLease(lease.ID))
				if err != nil {
					return err
				}
			}

			if grpcAddr != nil && flagEtcdGRPCPath != "" {
				var member balancedclient.GRPCMember
				member.Op = balancedclient.GRPCOpAdd
				member.Addr = grpcAddr.String()
				memberData, err := json.Marshal(&member)
				if err != nil {
					return err
				}

				grpcPath := flagEtcdGRPCPath + "/" + flagUnique
				_, err = etcd.KV.Put(ctx, grpcPath, string(memberData), v3.WithLease(lease.ID))
				if err != nil {
					return err
				}
			}

			return nil
		}

		shutdownEtcd = func() error {
			etcd.Lease.Revoke(ctx, lease.ID)
			etcd.Close()
			return nil
		}
	}

	advertiseATC := func() error { return nil }
	shutdownATC := func() error { return nil }
	if flagATCServers != "" {
		if flagATCName == "" || !regexp.MustCompile(`^[0-9A-Za-z]+(?:-[0-9A-Za-z]+)*$`).MatchString(flagATCName) {
			panic(fmt.Errorf("--atc-name: invalid name %q", flagATCName))
		}

		dialOpts := make([]grpc.DialOption, 1)
		if flagATCServerName == "" {
			dialOpts[0] = grpc.WithInsecure()
		} else {
			dialOpts[0] = grpc.WithTransportCredentials(
				credentials.NewTLS(
					&tls.Config{
						ServerName: flagATCServerName,
					}))
		}

		cc, err := grpc.DialContext(ctx, flagATCServers, dialOpts...)
		if err != nil {
			panic(fmt.Errorf("--atc-servers: failed to dial %q: %w", flagATCServers, err))
		}

		atcclient := roxypb.NewAirTrafficControlClient(cc)

		var rc roxypb.AirTrafficControl_ReportClient

		advertiseATC = func() error {
			var err error
			rc, err = atcclient.Report(ctx)
			if err != nil {
				return err
			}

			err = rc.Send(&roxypb.ReportRequest{
				Name:   flagATCName,
				Unique: flagUnique,
			})
			if err != nil {
				return err
			}

			// TODO: start thread to continuously report load

			return nil
		}

		shutdownATC = func() error {
			if rc != nil {
				rc.CloseAndRecv()
			}
			return nil
		}
	}

	sigch := make(chan os.Signal, 1)
	signal.Ignore(syscall.SIGHUP)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
	wg.Add(1)
	go func() {
		<-sigch
		signal.Stop(sigch)
		grpcShutdown(ctx)
		httpShutdown(ctx)
		shutdownATC()
		shutdownEtcd()
		shutdownZK()
		cancelfn()
		fmt.Println("shutdown")
		wg.Done()
	}()

	if err := advertiseZK(); err != nil {
		panic(err)
	}

	if err := advertiseEtcd(); err != nil {
		panic(err)
	}

	if err := advertiseATC(); err != nil {
		panic(err)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := httpListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := grpcListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	wg.Wait()
	fmt.Println("OK")
}

func parseHostPort(flagName string, hostPort string) (*net.TCPAddr, error) {
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to parse <host>:<port>: %w", flagName, err)
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to parse port number %q: %w", flagName, portStr, err)
	}
	if port == 0 {
		return nil, fmt.Errorf("%s: invalid port number 0", flagName)
	}

	var (
		ipStr string
		zone  string
	)
	if i := strings.IndexByte(host, '%'); i >= 0 {
		ipStr, zone = host[:i], host[i+1:]
	} else {
		ipStr = host
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("%s: failed to parse IP address %q", flagName, ipStr)
	}

	return &net.TCPAddr{IP: ip, Port: int(port), Zone: zone}, nil
}

func hostAndZone(addr *net.TCPAddr) string {
	if addr.Zone == "" {
		return addr.IP.String()
	}
	return addr.IP.String() + "%" + addr.Zone
}

type webServerServer struct {
	roxypb.UnimplementedWebServerServer
	corpus []byte
}

func (s *webServerServer) Serve(ws roxypb.WebServer_ServeServer) (err error) {
	hIn := make([]*roxypb.KeyValue, 0, 32)
	bIn := []byte(nil)
	schemeIn := ""
	methodIn := ""
	hostIn := ""
	pathIn := ""

	_ = schemeIn
	_ = hostIn

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
				hIn = append(hIn, kv)
				switch kv.Key {
				case ":scheme":
					schemeIn = kv.Value
				case ":method":
					methodIn = kv.Value
				case ":authority":
					hostIn = kv.Value
				case ":path":
					pathIn = kv.Value
				}
			}
		}
		if len(msg.Trailers) != 0 {
			for _, kv := range msg.Trailers {
				hIn = append(hIn, kv)
			}
		}
	}

	kvList(hIn).Sort()
	for _, kv := range hIn {
		fmt.Printf("[%s]: %s\n", kv.Key, kv.Value)
	}
	fmt.Print("\n")

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
