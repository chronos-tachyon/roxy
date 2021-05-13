// Command "colorize-logs" is a small command line tool that allows
// pretty-printing log files in JSON Lines format, with basic support for
// "tail -f" operation.
//
// Usage:
//
//	colorize-logs [<flags>] [<logfile>]
//
// Flags:
//
//	-f, --follow       keep reading the logfile for new log messages
//	-F, --follow-name  like --follow, but detect when the log file is rotated and re-open it
//
// If the <logfile> argument is omitted, then colorize-logs will read from
// stdin.
//
package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"
	"syscall"
	"time"

	getopt "github.com/pborman/getopt/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	flagFollow     bool
	flagFollowName bool
)

func init() {
	getopt.SetParameters("[<logfile>]")
	getopt.FlagLong(&flagFollow, "follow", 'f', "follow the log file in real time")
	getopt.FlagLong(&flagFollowName, "follow-name", 'F', "follow the log file in real time (with periodic checks for log rotation)")
}

func main() {
	getopt.Parse()

	var consoleWriter io.Writer = zerolog.ConsoleWriter{Out: os.Stdout}
	log.Logger = log.Output(consoleWriter)

	var mu sync.Mutex
	var inputFile *os.File
	wantClose := false
	defer func() {
		if wantClose {
			inputFile.Close()
		}
	}()

	inputFileName := "-"
	if getopt.NArgs() >= 1 {
		inputFileName = getopt.Arg(0)
	}

	if inputFileName == "-" {
		inputFile, wantClose = os.Stdin, false
	} else {
		var err error
		inputFile, err = os.Open(inputFileName)
		if err != nil {
			log.Logger.Fatal().
				Str("path", inputFileName).
				Err(err).
				Msg("failed to open input file")
		}
		wantClose = true
	}

	tryReopen := func() error { return nil }
	if flagFollowName && wantClose {
		tryReopen = func() error {
			inputFile2, err := os.Open(inputFileName)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}

			wantClose2 := true
			defer func() {
				if wantClose2 {
					inputFile2.Close()
				}
			}()

			fileInfo1, err := inputFile.Stat()
			if err != nil {
				return err
			}

			fileInfo2, err := inputFile2.Stat()
			if err != nil {
				return err
			}

			if stat1, ok1 := fileInfo1.Sys().(*syscall.Stat_t); ok1 {
				if stat2, ok2 := fileInfo2.Sys().(*syscall.Stat_t); ok2 {
					if stat1.Dev != stat2.Dev || stat1.Ino != stat2.Ino {
						mu.Lock()
						inputFile, inputFile2 = inputFile2, inputFile
						mu.Unlock()
					}
				}
			}
			wantClose2 = false
			return inputFile2.Close()
		}
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

		var counter uint

	Top:
		counter = 0
		for {
			mu.Lock()
			f := inputFile
			mu.Unlock()
			buf := make([]byte, 4096)
			n, err := f.Read(buf)
			if n > 0 {
				ch1 <- buf[:n]
			}
			if err != nil && err != io.EOF {
				log.Logger.Error().
					Err(err).
					Msg("failed to read from input file")
				break
			}
			if err == io.EOF && !flagFollow && !flagFollowName {
				break
			}
			if err == io.EOF {
				counter++
				time.Sleep(250 * time.Millisecond)
				if counter >= 8 {
					counter = 0
					if e := tryReopen(); e != nil {
						log.Logger.Warn().
							Str("path", inputFileName).
							Err(err).
							Msg("failed to re-open input file")
					}
				}
			}
		}
		if flagFollowName {
			goto Top
		}
	}()

	wg.Add(1)
	go func() {
		defer func() {
			close(ch2)
			wg.Done()
		}()

		var buf []byte
		var scrapLen int
		var scraps [][]byte

		tryFlush := func() bool {
			if len(buf) == 0 {
				return false
			}

			i := bytes.IndexByte(buf, '\n')
			if i < 0 {
				scrapLen += len(buf)
				scraps = append(scraps, buf)
				buf = nil
				return false
			}

			tmp := make([]byte, 0, scrapLen+i+1)
			for _, scrap := range scraps {
				tmp = append(tmp, scrap...)
			}
			tmp = append(tmp, buf[:i+1]...)
			buf = buf[i+1:]
			scrapLen = 0
			scraps = nil
			ch2 <- string(tmp)
			return true
		}

		for {
			var ok bool
			buf, ok = <-ch1
			if !ok {
				break
			}

			for tryFlush() {
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
		_, _ = consoleWriter.Write([]byte(line))
	}
	wg.Wait()
}
