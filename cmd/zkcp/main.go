package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	zkclient "github.com/go-zookeeper/zk"
	getopt "github.com/pborman/getopt/v2"
)

var (
	flagServers  string
	flagUsername string
	flagPassword string
)

func init() {
	getopt.SetParameters("<src-local-path> <dst-zookeeper-path>")
	getopt.FlagLong(&flagServers, "servers", 's', "comma-separated list of zookeeper servers")
	getopt.FlagLong(&flagUsername, "username", 'u', "auth username")
	getopt.FlagLong(&flagPassword, "password", 'p', "auth password")
}

func main() {
	getopt.Parse()

	if flagServers == "" {
		fmt.Fprintf(os.Stderr, "fatal: missing required flag \"-s\" / \"--servers\"\n")
		os.Exit(1)
	}

	if getopt.NArgs() != 2 {
		fmt.Fprintf(os.Stderr, "fatal: wrong number of positional arguments: expected 2, got %d\n", getopt.NArgs())
		os.Exit(1)
	}

	servers := strings.Split(flagServers, ",")
	localPath := getopt.Arg(0)
	zkPath := getopt.Arg(1)

	data, err := ioutil.ReadFile(localPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: failed to read local file %q: %v\n", localPath, err)
		os.Exit(1)
	}

	zk, _, err := zkclient.Connect(servers, 30*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer zk.Close()

	if flagUsername != "" && flagPassword != "" {
		scheme := "digest"
		raw := []byte(flagUsername + ":" + flagPassword)
		err = zk.AddAuth(scheme, raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: failed to auth: %v\n", err)
			os.Exit(1)
		}
	}

	_, err = zk.Create(zkPath, data, 0, zkclient.WorldACL(zkclient.PermAll))
	if err == zkclient.ErrNodeExists {
		_, err = zk.Set(zkPath, data, -1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: failed to create/set zookeeper path %q: %v\n", zkPath, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "%q => %q: OK\n", localPath, zkPath)
}
