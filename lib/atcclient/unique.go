package atcclient

import (
	"errors"
	"io/fs"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/rs/xid"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

var (
	gUniqueFile string
	gUniqueOnce sync.Once
	gUniqueID   string
	gUniqueErr  error
)

// SetUniqueFile specifies the path to the state file where UniqueID is stored.
func SetUniqueFile(path string) {
	abs, err := roxyutil.ExpandPath(path)
	if err != nil {
		panic(err)
	}
	gUniqueFile = abs
}

// HasUniqueID returns true iff the ATC Unique ID could be loaded or persisted.
//
// The program must call SetUniqueFile in main() before calling this function.
func HasUniqueID() bool {
	gUniqueOnce.Do(initUniqueID)
	return (gUniqueErr == nil)
}

// UniqueID returns the ATC Unique ID associated with this program.
//
// The program must call SetUniqueFile in main() before calling this function.
func UniqueID() (string, error) {
	gUniqueOnce.Do(initUniqueID)
	return gUniqueID, gUniqueErr
}

func initUniqueID() {
	if gUniqueFile == "" {
		gUniqueID = ""
		gUniqueErr = errors.New("must call mainutil.SetUniqueFile")
		return
	}

	for {
		str, err := loadUniqueID()
		if err == nil {
			gUniqueID = str
			gUniqueErr = nil
			return
		}
		if !errors.Is(err, fs.ErrNotExist) {
			gUniqueID = ""
			gUniqueErr = err
			return
		}

		str, err = generateUniqueID()
		if err == nil {
			gUniqueID = str
			gUniqueErr = nil
			return
		}
		if !errors.Is(err, fs.ErrExist) {
			gUniqueID = ""
			gUniqueErr = err
			return
		}
	}
}

func loadUniqueID() (string, error) {
	raw, err := ioutil.ReadFile(gUniqueFile)
	if err != nil {
		return "", err
	}

	str := strings.Trim(string(raw), " \t\r\n")
	err = roxyutil.ValidateATCUniqueID(str)
	if err != nil {
		return "", err
	}

	return str, nil
}

func generateUniqueID() (string, error) {
	str := xid.New().String()
	raw := []byte(str + "\n")

	f, err := os.OpenFile(gUniqueFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
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

	return str, nil
}
