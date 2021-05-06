package main

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/chronos-tachyon/roxy/internal/enums"
	"github.com/chronos-tachyon/roxy/roxypb"
)

// type fileInfoList {{{

type fileInfoList []fs.FileInfo

func (list fileInfoList) Len() int {
	return len(list)
}

func (list fileInfoList) Less(i, j int) bool {
	a, b := list[i], list[j]

	aName, aIsDir := a.Name(), a.IsDir()
	bName, bIsDir := b.Name(), b.IsDir()

	aIsDot := strings.HasPrefix(aName, ".")
	bIsDot := strings.HasPrefix(bName, ".")

	if aIsDir != bIsDir {
		return aIsDir
	}
	if aIsDot != bIsDot {
		return aIsDot
	}
	return aName < bName
}

func (list fileInfoList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

var _ sort.Interface = fileInfoList(nil)

// }}}

func simplifyHTTPMethod(str string) string {
	switch str {
	case http.MethodOptions:
		return http.MethodOptions
	case http.MethodGet:
		return http.MethodGet
	case http.MethodHead:
		return http.MethodHead
	case http.MethodPost:
		return http.MethodPost
	case http.MethodPut:
		return http.MethodPut
	case http.MethodPatch:
		return http.MethodPatch
	case http.MethodDelete:
		return http.MethodDelete
	default:
		return "OTHER"
	}
}

func simplifyHTTPStatusCode(statusCode int) string {
	switch statusCode {
	case 100:
		return "100"
	case 101:
		return "101"
	case 0, 200:
		return "200"
	case 204:
		return "204"
	case 206:
		return "206"
	case 301:
		return "301"
	case 302:
		return "302"
	case 304:
		return "304"
	case 307:
		return "307"
	case 308:
		return "308"
	case 400:
		return "400"
	case 401:
		return "401"
	case 403:
		return "403"
	case 404:
		return "404"
	case 405:
		return "405"
	case 500:
		return "500"
	case 503:
		return "503"
	}

	switch {
	case statusCode < 200:
		return "1xx"
	case statusCode < 300:
		return "2xx"
	case statusCode < 400:
		return "3xx"
	case statusCode < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

func simplifyTargetKey(key string) string {
	switch {
	case strings.HasPrefix(key, "ERROR:"):
		return "!ERROR"
	case strings.HasPrefix(key, "REDIR:"):
		return "!REDIR"
	default:
		return key
	}
}

func toHTTPError(err error) int {
	switch {
	case os.IsNotExist(err):
		return http.StatusNotFound
	case os.IsPermission(err):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func runeLen(str string) int {
	var length int
	for _, ch := range str {
		_ = ch
		length++
	}
	return length
}

func trimContentHeader(str string) string {
	i := strings.IndexByte(str, ';')
	if i >= 0 {
		str = str[:i]
	}
	return strings.TrimSpace(str)
}

func hexToBase64(in string) string {
	raw, err := hex.DecodeString(in)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func appendHeadersToKV(kv []*roxypb.KeyValue, h http.Header) []*roxypb.KeyValue {
	list := make([]string, 0, len(h))
	valueMap := make(map[string][]string, len(h))
	for key, values := range h {
		name := strings.ToLower(key)
		list = append(list, name)
		valueMap[name] = values
	}
	sort.Strings(list)
	for _, name := range list {
		for _, value := range valueMap[name] {
			kv = append(kv, &roxypb.KeyValue{Key: name, Value: value})
		}
	}
	return kv
}

func readXattr(f http.File, attr string) ([]byte, error) {
	fdable, ok := f.(interface{ Fd() uintptr })
	if !ok {
		return nil, syscall.ENOTSUP
	}

	dest := make([]byte, 256)
	for {
		n, err := unix.Fgetxattr(int(fdable.Fd()), attr, dest)
		switch {
		case err == nil && n < len(dest):
			return dest[:n], nil

		case err == nil:
			if len(dest) >= 65536 {
				return nil, syscall.ERANGE
			}
			dest = make([]byte, 2*len(dest))

		case errors.Is(err, syscall.ERANGE):
			if len(dest) >= 65536 {
				return nil, err
			}
			dest = make([]byte, 2*len(dest))

		case errors.Is(err, syscall.EINTR):
			// pass

		default:
			return nil, err
		}
	}
}

func readLinkAt(dir http.File, name string) (string, error) {
	fdable, ok := dir.(interface{ Fd() uintptr })
	if !ok {
		return "", syscall.ENOTSUP
	}

	dest := make([]byte, 256)
	for {
		n, err := unix.Readlinkat(int(fdable.Fd()), name, dest)
		switch {
		case err == nil && n < len(dest):
			return string(dest[:n]), nil

		case err == nil:
			if len(dest) >= 65536 {
				return "", syscall.ERANGE
			}
			dest = make([]byte, 2*len(dest))

		case errors.Is(err, syscall.EINTR):
			// pass

		default:
			return "", err
		}
	}
}

func setDigestHeader(h http.Header, algo enums.DigestType, b64 string) {
	h.Add("digest", fmt.Sprintf("%s=%s", algo, b64))
}

func setETagHeader(h http.Header, prefix string, lastMod time.Time) {
	if h.Get("etag") != "" {
		return
	}

	var (
		haveMD5    bool
		haveSHA1   bool
		haveSHA256 bool
		sumMD5     string
		sumSHA1    string
		sumSHA256  string
	)

	for _, row := range h.Values("digest") {
		switch {
		case strings.HasPrefix(row, "md5="):
			haveMD5 = true
			sumMD5 = row[4:]
		case strings.HasPrefix(row, "sha1="):
			haveSHA1 = true
			sumSHA1 = row[5:]
		case strings.HasPrefix(row, "sha256="):
			haveSHA256 = true
			sumSHA256 = row[7:]
		}
	}

	if haveSHA256 {
		h.Set("etag", strconv.Quote(prefix+"Z."+sumSHA256[:16]))
		return
	}

	if haveSHA1 {
		h.Set("etag", strconv.Quote(prefix+"Y."+sumSHA1[:16]))
		return
	}

	if haveMD5 {
		h.Set("etag", strconv.Quote(prefix+"X."+sumMD5[:16]))
		return
	}

	h.Set("etag", fmt.Sprintf("W/%q", lastMod.UTC().Format("2006.01.02.15.04.05")))
}

func addrWithNoPort(addr net.Addr) string {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		if tcpAddr.Zone == "" {
			return tcpAddr.IP.String()
		}
		return tcpAddr.IP.String() + "%" + tcpAddr.Zone
	}
	return addr.String()
}

func quoteForwardedAddr(addr net.Addr) string {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		str := tcpAddr.IP.String()
		if tcpAddr.Zone != "" {
			str += "%" + tcpAddr.Zone
		}
		if tcpAddr.Zone != "" || isIPv6(tcpAddr.IP) {
			str = strconv.Quote(str)
		}
		return str
	}
	return addr.String()
}

func isIPv6(ip net.IP) bool {
	if len(ip) < 16 {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		return false
	}
	return true
}
