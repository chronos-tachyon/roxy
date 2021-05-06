package main

import (
	"io/ioutil"

	"github.com/go-zookeeper/zk"
	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/lib/mainutil"
)

var (
	flagZK      string = "127.0.0.1:2181"
	flagReverse bool
)

func init() {
	getopt.SetParameters("<src> <dest>")
	getopt.FlagLong(&flagZK, "zk", 'Z', "ZooKeeper client configuration")
	getopt.FlagLong(&flagReverse, "reverse", 'r', "copy from ZooKeeper to filesystem, instead of filesystem to ZooKeeper")
}

func main() {
	getopt.Parse()

	mainutil.InitContext()
	defer mainutil.CancelRootContext()
	ctx := mainutil.RootContext()

	mainutil.InitLogging()
	defer mainutil.DoneLogging()

	var zc mainutil.ZKConfig
	err := zc.Parse(flagZK)
	if err != nil {
		log.Logger.Fatal().
			Str("input", flagZK).
			Err(err).
			Msg("--zk: failed to parse")
	}

	if getopt.NArgs() != 2 {
		log.Logger.Fatal().
			Int("expected", 2).
			Int("actual", getopt.NArgs()).
			Msg("wrong number of positional arguments")
	}
	srcPath := getopt.Arg(0)
	dstPath := getopt.Arg(1)

	zkconn, err := zc.Connect(ctx)
	if err != nil {
		log.Logger.Fatal().
			Interface("config", zc).
			Err(err).
			Msg("--zk: failed to connect")
	}
	defer zkconn.Close()

	if flagReverse {
		data, _, err := zkconn.Get(srcPath)
		if err != nil {
			log.Logger.Fatal().
				Str("path", srcPath).
				Err(err).
				Msg("failed to read ZooKeeper node")
		}

		err = ioutil.WriteFile(dstPath, data, 0666)
		if err != nil {
			log.Logger.Fatal().
				Str("path", dstPath).
				Err(err).
				Msg("failed to write filesystem file")
		}
	} else {
		data, err := ioutil.ReadFile(srcPath)
		if err != nil {
			log.Logger.Fatal().
				Str("path", srcPath).
				Err(err).
				Msg("failed to read filesystem file")
		}

		_, err = zkconn.Create(dstPath, data, 0, zk.WorldACL(zk.PermAll))
		if err == zk.ErrNodeExists {
			_, err = zkconn.Set(dstPath, data, -1)
		}
		if err != nil {
			log.Logger.Fatal().
				Str("path", dstPath).
				Err(err).
				Msg("failed to Create or Set ZooKeeper node")
		}
	}

	log.Logger.Info().
		Str("source", srcPath).
		Str("destination", dstPath).
		Msg("OK")
}
