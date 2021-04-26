package main

import (
	"bytes"
	"io"
	"os"
	"sync"
	"time"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var flagFollow bool

func init() {
	getopt.SetParameters("[<logfile>]")
	getopt.FlagLong(&flagFollow, "follow", 'f', "follow the log file in real time")
}

func main() {
	getopt.Parse()

	var consoleWriter io.Writer = zerolog.ConsoleWriter{Out: os.Stdout}
	log.Logger = log.Output(consoleWriter)

	var (
		f         *os.File
		wantClose bool
	)

	defer func() {
		if wantClose {
			f.Close()
		}
	}()

	inputFile := "-"
	if getopt.NArgs() >= 1 {
		inputFile = getopt.Arg(0)
	}

	if inputFile == "-" {
		f, wantClose = os.Stdin, false
	} else {
		var err error
		f, err = os.Open(inputFile)
		if err != nil {
			log.Logger.Fatal().
				Str("inputFile", inputFile).
				Err(err).
				Msg("failed to open input file")
		}
		wantClose = true
	}

	ch1 := make(chan []byte)
	ch2 := make(chan string)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer func() {
			close(ch1)
			wg.Done()
		}()

		for {
			buf := make([]byte, 4096)
			n, err := f.Read(buf)
			if n > 0 {
				ch1 <- buf[:n]
			}
			if err != nil && err != io.EOF {
				log.Logger.Error().
					Err(err).
					Msg("failed to read from input file")
				return
			}
			if err == io.EOF && !flagFollow {
				return
			}
			if err == io.EOF {
				time.Sleep(250 * time.Millisecond)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer func() {
			close(ch2)
			wg.Done()
		}()

		var scraps [][]byte
		var scrapLen int
		for buf := range ch1 {
			i := bytes.IndexByte(buf, '\n')
			if i < 0 {
				scraps = append(scraps, buf)
				scrapLen += len(buf)
				continue
			}

			tmp := make([]byte, 0, scrapLen+i+1)
			for _, scrap := range scraps {
				tmp = append(tmp, scrap...)
			}
			tmp = append(tmp, buf[:i+1]...)
			buf = buf[i+1:]
			scraps = nil
			scrapLen = 0

			ch2 <- string(tmp)

			if len(buf) != 0 {
				scraps = append(scraps, buf)
				scrapLen += len(buf)
			}
		}
		if scrapLen != 0 {
			tmp := make([]byte, 0, scrapLen)
			for _, scrap := range scraps {
				tmp = append(tmp, scrap...)
			}
			ch2 <- string(tmp)
		}
	}()

	for line := range ch2 {
		consoleWriter.Write([]byte(line))
	}
	wg.Wait()
}
