package mainutil

import (
	"errors"
	"io/fs"
	"io/ioutil"
	"os"
	"strings"

	"github.com/rs/xid"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

var gUniqueFile string

// SetUniqueFile specifies the path to the state file where UniqueID is stored.
func SetUniqueFile(path string) {
	abs, err := roxyutil.ExpandPath(path)
	if err != nil {
		panic(err)
	}
	gUniqueFile = abs
}

// UniqueID returns the ATC Unique ID associated with this program.
//
// The program must call SetUniqueFile in main() before calling this function.
func UniqueID() (string, error) {
	if gUniqueFile == "" {
		return "", errors.New("must call mainutil.SetUniqueFile")
	}

	for {
		raw, err := ioutil.ReadFile(gUniqueFile)
		if err == nil {
			unique := strings.Trim(string(raw), " \t\r\n")
			return unique, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		unique := xid.New().String()
		raw = []byte(unique + "\n")

		f, err := os.OpenFile(gUniqueFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}

		_, err = f.Write(raw)
		if err != nil {
			_ = f.Close()
			return "", err
		}

		err = f.Close()
		if err != nil {
			return "", err
		}

		return unique, nil
	}
}
