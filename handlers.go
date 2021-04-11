package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	zerolog "github.com/rs/zerolog"
	log "github.com/rs/zerolog/log"
	unix "golang.org/x/sys/unix"
)

const (
	textNotFound  = "404 Not Found"
	textForbidden = "403 Forbidden"
	textISE       = "500 Internal Server Error"

	xattrMimeType  = "user.mimetype"
	xattrMd5sum    = "user.md5sum"
	xattrSha1sum   = "user.sha1sum"
	xattrSha256sum = "user.sha256sum"
	xattrEtag      = "user.etag"

	defaultMaxCacheSize = 1 << 16 // 64 KiB
)

var (
	gFileCacheMu  sync.Mutex
	gFileCacheMap map[cacheKey]cacheRow = make(map[cacheKey]cacheRow, 1024)
)

// type BasicSecurityHandler {{{

type BasicSecurityHandler struct {
	Ref  *Ref
	Next http.Handler
}

func (h BasicSecurityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.Header().Set("Strict-Transport-Security", "max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	h.Next.ServeHTTP(w, r)
}

var _ http.Handler = BasicSecurityHandler{}

// }}}

// type LoggingHandler {{{

type LoggingHandler struct {
	RootLogger *zerolog.Logger
	Service    string
	Next       http.Handler
}

func (h LoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := h.RootLogger.With()
	c = c.Str("service", h.Service)
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		c = c.Str("ip", host)
	}
	c = c.Str("method", r.Method)
	c = c.Str("host", r.Host)
	c = c.Str("url", r.URL.String())
	if value := r.Header.Get("user-agent"); value != "" {
		c = c.Str("userAgent", value)
	}
	if value := r.Header.Get("referer"); value != "" {
		c = c.Str("referer", value)
	}

	logger := c.Logger()
	r = r.WithContext(logger.WithContext(r.Context()))

	ww := WrapWriter(w)

	logger.Debug().Msg("start request")
	startTime := time.Now()

	defer func() {
		panicValue := recover()
		endTime := time.Now()
		elapsed := endTime.Sub(startTime)

		if panicValue != nil {
			http.Error(ww, textISE, http.StatusInternalServerError)
		}

		status := ww.Status()
		bytesWritten := ww.BytesWritten()
		location := ww.Header().Get("location")
		contentType := ww.Header().Get("content-type")

		var event *zerolog.Event
		if panicValue != nil {
			event = logger.Error()
			switch x := panicValue.(type) {
			case error:
				event = event.AnErr("panic", x)
			case string:
				event = event.Str("panic", x)
			default:
				event = event.Interface("panic", x)
			}
		} else {
			event = logger.Info()
		}
		event = event.Dur("elapsed", elapsed)
		event = event.Int("status", status)
		event = event.Int64("bytesWritten", bytesWritten)
		if location != "" {
			event = event.Str("location", location)
		}
		if contentType != "" {
			event = event.Str("contentType", contentType)
		}
		event.Msg("end request")
	}()

	h.Next.ServeHTTP(ww, r)
}

var _ http.Handler = LoggingHandler{}

// }}}

// type ErrorHandler {{{

type ErrorHandler struct {
	status int
}

func (h ErrorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "", h.status)
}

var _ http.Handler = ErrorHandler{}

// }}}

// type RedirHandler {{{

type RedirHandler struct {
	tmpl   *template.Template
	status int
}

func (h RedirHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	r.URL.Scheme = "https"
	r.URL.Host = r.Host

	var buf strings.Builder
	err := h.tmpl.Execute(&buf, r.URL)
	if err != nil {
		logger.Warn().
			Str("type", "redir").
			Interface("url", r.URL).
			Msg("failed to execute template")
		http.Error(w, textISE, http.StatusInternalServerError)
		return
	}

	urlStr := buf.String()
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		logger.Warn().
			Str("type", "redir").
			Str("url", urlStr).
			Msg("invalid url")
		http.Error(w, textISE, http.StatusInternalServerError)
		return
	}

	urlStr2 := urlObj.String()
	http.Redirect(w, r, urlStr2, h.status)
}

var _ http.Handler = RedirHandler{}

// }}}

// type FileSystemHandler {{{

type FileSystemHandler struct {
	impl *Impl
	key  string
	fs   http.FileSystem
}

func (h FileSystemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Path
	if !strings.HasPrefix(reqPath, "/") {
		reqPath = "/" + reqPath
	}
	reqPath = path.Clean(reqPath)

	if path.Base(reqPath) == "index.html" {
		reqPath = path.Dir(reqPath)
		if !strings.HasSuffix(reqPath, "/") {
			reqPath += "/"
		}
		http.Redirect(w, r, reqPath, http.StatusFound)
		return
	}

	reqFile, err := h.fs.Open(reqPath)
	if err != nil {
		msg, code := toHTTPError(err)
		http.Error(w, msg, code)
		return
	}

	defer reqFile.Close()

	reqStat, err := reqFile.Stat()
	if err != nil {
		http.Error(w, textISE, http.StatusInternalServerError)
		return
	}

	if reqStat.IsDir() && !strings.HasSuffix(r.URL.Path, "/") {
		reqPath += "/"
		http.Redirect(w, r, reqPath, http.StatusFound)
		return
	}

	if !reqStat.IsDir() && strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, reqPath, http.StatusFound)
		return
	}

	if reqStat.IsDir() {
		idxPath := filepath.Join(reqPath, "index.html")
		idxFile, err := h.fs.Open(idxPath)
		switch {
		case err == nil:
			defer func() {
				if idxFile != nil {
					idxFile.Close()
				}
			}()

			idxStat, err := idxFile.Stat()
			if err != nil {
				http.Error(w, textISE, http.StatusInternalServerError)
				return
			}

			if !idxStat.IsDir() {
				reqPath, reqFile, reqStat = idxPath, idxFile, idxStat
				idxFile, idxStat = nil, nil
			}

		case os.IsNotExist(err):
			// pass

		default:
			msg, code := toHTTPError(err)
			http.Error(w, msg, code)
			return
		}
	}

	r.URL.Path = reqPath

	if reqStat.IsDir() {
		h.ServeDir(w, r, reqFile, reqStat)
		return
	}

	h.ServeFile(w, r, reqFile, reqStat)
}

func (h FileSystemHandler) ServeFile(w http.ResponseWriter, r *http.Request, f http.File, fi fs.FileInfo) {
	logger := log.Ctx(r.Context())
	hdrs := w.Header()

	var (
		cachePossible   bool
		cacheHit        bool
		haveContentType bool
		haveMd5sum      bool
		haveSha1sum     bool
		haveSha256sum   bool
		haveEtag        bool
		tookSlowPath    bool
		body            io.ReadSeeker = f
	)

	hdrs.Set("content-type", "application/octet-stream")

	if raw, err := readXattr(f, xattrMimeType); err == nil {
		hdrs.Set("content-type", string(raw))
		haveContentType = true
	}

	for _, mimeRule := range h.impl.mimeRules {
		if !mimeRule.rx.MatchString(r.URL.Path) {
			continue
		}
		if !haveContentType && mimeRule.contentType != "" {
			hdrs.Set("content-type", mimeRule.contentType)
			haveContentType = true
		}
	}

	size := fi.Size()
	mtime := fi.ModTime()
	if size <= defaultMaxCacheSize {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			cachePossible = true
			key := cacheKey{st.Dev, st.Ino}

			gFileCacheMu.Lock()
			row, found := gFileCacheMap[key]
			gFileCacheMu.Unlock()

			if found && size == int64(len(row.bytes)) && mtime.Equal(row.mtime) {
				cacheHit = true
			} else {
				raw, err := ioutil.ReadAll(f)
				if err != nil {
					logger.Error().
						Err(err).
						Msg("failed to read file")
					http.Error(w, textISE, http.StatusInternalServerError)
					return
				}

				size = int64(len(raw))
				md5raw := md5.Sum(raw)
				sha1raw := sha1.Sum(raw)
				sha256raw := sha256.Sum256(raw)

				md5sum := hex.EncodeToString(md5raw[:])
				sha1sum := hex.EncodeToString(sha1raw[:])
				sha256sum := hex.EncodeToString(sha256raw[:])
				etag := strconv.Quote("Z" + sha256sum[:16])

				row = cacheRow{
					bytes:     raw,
					mtime:     mtime,
					etag:      etag,
					md5sum:    md5sum,
					sha1sum:   sha1sum,
					sha256sum: sha256sum,
				}

				gFileCacheMu.Lock()
				gFileCacheMap[key] = row
				gFileCacheMu.Unlock()
			}

			hdrs.Set("content-md5", row.md5sum)
			hdrs.Set("content-sha1", row.sha1sum)
			hdrs.Set("content-sha256", row.sha256sum)
			hdrs.Set("etag", row.etag)
			body = bytes.NewReader(row.bytes)
			haveMd5sum = true
			haveSha1sum = true
			haveSha256sum = true
			haveEtag = true
		}
	}

	if !cachePossible {
		if raw, err := readXattr(f, xattrMd5sum); err == nil {
			md5sum := string(raw)
			hdrs.Set("content-md5", md5sum)
			haveMd5sum = true
		}

		if raw, err := readXattr(f, xattrSha1sum); err == nil {
			sha1sum := string(raw)
			hdrs.Set("content-sha1", sha1sum)
			haveSha1sum = true
		}

		if raw, err := readXattr(f, xattrSha256sum); err == nil {
			sha256sum := string(raw)
			hdrs.Set("content-sha256", sha256sum)
			haveSha256sum = true
		}

		if raw, err := readXattr(f, xattrEtag); err == nil {
			etag := string(raw)
			hdrs.Set("etag", etag)
			haveEtag = true
		}

		if !haveMd5sum && !haveSha1sum && !haveSha256sum {
			if _, err := f.Seek(0, io.SeekStart); err == nil {
				md5writer := md5.New()
				sha1writer := sha1.New()
				sha256writer := sha256.New()

				mw := io.MultiWriter(md5writer, sha1writer, sha256writer)
				size, err = io.Copy(mw, f)
				if err != nil {
					logger.Error().Err(err).Msg("failed to read file contents")
					http.Error(w, textISE, http.StatusInternalServerError)
					return
				}

				_, err = f.Seek(0, io.SeekStart)
				if err != nil {
					logger.Error().Err(err).Msg("failed to seek to beginning")
					http.Error(w, textISE, http.StatusInternalServerError)
					return
				}

				md5sum := hex.EncodeToString(md5writer.Sum(nil))
				sha1sum := hex.EncodeToString(sha1writer.Sum(nil))
				sha256sum := hex.EncodeToString(sha256writer.Sum(nil))

				hdrs.Set("content-md5", md5sum)
				hdrs.Set("content-sha1", sha1sum)
				hdrs.Set("content-sha256", sha256sum)
				haveMd5sum = true
				haveSha1sum = true
				haveSha256sum = true
				tookSlowPath = true
			}
		}
	}

	if !haveEtag {
		switch {
		case haveSha256sum:
			sha256sum := hdrs.Get("content-sha256")
			etag := strconv.Quote("Z" + sha256sum[:16])
			hdrs.Set("etag", etag)
			haveEtag = true

		case haveSha1sum:
			sha1sum := hdrs.Get("content-sha1")
			etag := strconv.Quote("Y" + sha1sum[:16])
			hdrs.Set("etag", etag)
			haveEtag = true

		case haveMd5sum:
			md5sum := hdrs.Get("content-md5")
			etag := strconv.Quote("X" + md5sum[:16])
			hdrs.Set("etag", etag)
			haveEtag = true
		}
	}

	logger.Debug().
		Bool("cachePossible", cachePossible).
		Bool("cacheHit", cacheHit).
		Bool("haveContentType", haveContentType).
		Bool("haveMd5sum", haveMd5sum).
		Bool("haveSha1sum", haveSha1sum).
		Bool("haveSha256sum", haveSha256sum).
		Bool("haveEtag", haveEtag).
		Bool("tookSlowPath", tookSlowPath).
		Msg("serve file")
	http.ServeContent(w, r, fi.Name(), mtime, body)
}

func (h FileSystemHandler) ServeDir(w http.ResponseWriter, r *http.Request, f http.File, fi fs.FileInfo) {
	logger := log.Ctx(r.Context())
	hdrs := w.Header()

	var entries anyDirs
	var err error
	if x, ok := f.(fs.ReadDirFile); ok {
		var list dirEntryDirs
		list, err = x.ReadDir(-1)
		entries = list
	} else {
		var list fileInfoDirs
		list, err = f.Readdir(-1)
		entries = list
	}

	if err != nil {
		logger.Error().Err(err).Msg("failed to read directory")
		http.Error(w, textISE, http.StatusInternalServerError)
		return
	}

	sort.Sort(entries)

	var buf strings.Builder
	buf.WriteString("<!DOCTYPE html>")
	buf.WriteString("<html>\n")
	buf.WriteString("\t<head>\n")
	buf.WriteString("\t\t<meta charset=\"utf-8\">\n")
	buf.WriteString("\t\t<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	buf.WriteString("\t\t<title>Listing of ")
	buf.WriteString(html.EscapeString(r.URL.Path))
	buf.WriteString("</title>\n")
	buf.WriteString("\t</head>\n")
	buf.WriteString("\t<body>\n")
	buf.WriteString("\t\t<pre>")
	for i, j := 0, entries.Len(); i < j; i++ {
		name := entries.Name(i)
		isDir := entries.IsDir(i)

		mode := "-rw-r--r--"
		slash := ""
		if isDir {
			mode = "drwxr-xr-x"
			slash = "/"
		}

		buf.WriteString(mode)
		buf.WriteByte(' ')
		buf.WriteString("<a href=\"")
		buf.WriteString(html.EscapeString(url.PathEscape(name)))
		buf.WriteString(slash)
		buf.WriteString("\">")
		buf.WriteString(html.EscapeString(name))
		buf.WriteString(slash)
		buf.WriteString("</a>\n")
	}
	buf.WriteString("</pre>\n")
	buf.WriteString("\t</body>\n")
	buf.WriteString("</html>\n")

	raw := []byte(buf.String())

	md5raw := md5.Sum(raw)
	sha1raw := sha1.Sum(raw)
	sha256raw := sha256.Sum256(raw)

	md5sum := hex.EncodeToString(md5raw[:])
	sha1sum := hex.EncodeToString(sha1raw[:])
	sha256sum := hex.EncodeToString(sha256raw[:])
	etag := strconv.Quote("D" + sha256sum[:16])

	hdrs.Set("content-type", "text/html; charset=utf-8")
	hdrs.Set("content-md5", md5sum)
	hdrs.Set("content-sha1", sha1sum)
	hdrs.Set("content-sha256", sha256sum)
	hdrs.Set("etag", etag)

	logger.Debug().
		Msg("serve directory")
	http.ServeContent(w, r, "", fi.ModTime(), bytes.NewReader(raw))
}

var _ http.Handler = FileSystemHandler{}

// }}}

// type BackendHandler {{{

type BackendHandler struct {
	impl   *Impl
	key    string
	proto  string
	addr   string
	client *http.Client
}

func (h BackendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.Ctx(r.Context())

	innerURL := new(url.URL)
	*innerURL = *r.URL
	innerURL.Scheme = "http"
	innerURL.Host = "0.0.0.0"

	innerReq, err := http.NewRequestWithContext(
		ctx, r.Method, innerURL.String(), r.Body)
	if err != nil {
		panic(err)
	}

	innerReq.Header = make(http.Header, len(r.Header))
	for key, oldValues := range r.Header {
		newValues := make([]string, len(oldValues))
		copy(newValues, oldValues)
		innerReq.Header[key] = newValues
	}
	innerReq.Header.Set("host", r.Host)
	innerReq.Host = r.Host
	innerReq.Close = true

	innerResp, err := h.client.Do(innerReq)
	if err != nil {
		logger.Error().
			Str("proto", h.proto).
			Str("addr", h.addr).
			Err(err).
			Msg("failed subrequest")
		http.Error(w, textISE, http.StatusInternalServerError)
		return
	}

	needBodyClose := true
	defer func() {
		if needBodyClose {
			innerResp.Body.Close()
		}
	}()

	hdrs := w.Header()
	for key, oldValues := range innerResp.Header {
		newValues := make([]string, len(oldValues))
		copy(newValues, oldValues)
		hdrs[key] = newValues
	}
	w.WriteHeader(innerResp.StatusCode)

	_, err = io.Copy(w, innerResp.Body)
	if err != nil {
		logger.Error().
			Str("proto", h.proto).
			Str("addr", h.addr).
			Err(err).
			Msg("failed subrequest")
	}

	needBodyClose = false
	err = innerResp.Body.Close()
	if err != nil {
		logger.Warn().
			Str("proto", h.proto).
			Str("addr", h.addr).
			Err(err).
			Msg("failed to close body of subrequest")
	}
}

var _ http.Handler = BackendHandler{}

// }}}

func CompileErrorHandler(impl *Impl, arg string) (http.Handler, error) {
	// format: "ERROR:<status>"

	statusStr := arg[6:]

	statusU64, err := strconv.ParseUint(statusStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uint %q: %w", statusStr, err)
	}
	if statusU64 < 400 || statusU64 >= 600 {
		return nil, fmt.Errorf("status %d out of range [400..599]", statusU64)
	}

	return ErrorHandler{int(statusU64)}, nil
}

func CompileRedirHandler(impl *Impl, arg string) (http.Handler, error) {
	// format: "REDIR:<status>:https://example.com/path?query"

	statusStr := arg[6:9]
	templateStr := arg[10:]

	statusU64, err := strconv.ParseUint(statusStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uint %q: %w", statusStr, err)
	}
	if statusU64 < 300 || statusU64 >= 400 {
		return nil, fmt.Errorf("status %d out of range [300..399]", statusU64)
	}

	templateObj := template.New("redir")
	templateObj, err = templateObj.Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to compile text/template %q: %w", templateStr, err)
	}
	return RedirHandler{templateObj, int(statusU64)}, nil
}

func CompileTarget(impl *Impl, key string, cfg *TargetConfig) (http.Handler, error) {
	switch cfg.Type {
	case UndefinedTargetType:
		return nil, fmt.Errorf("missing required field \"type\"")

	case FileSystemTargetType:
		return CompileFileSystemHandler(impl, key, cfg)

	case BackendTargetType:
		return CompileBackendHandler(impl, key, cfg)

	default:
		panic(fmt.Errorf("unknown target type %s(%d)", cfg.Type, cfg.Type))
	}
}

func CompileFileSystemHandler(impl *Impl, key string, cfg *TargetConfig) (http.Handler, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("missing required field \"path\"")
	}

	abs, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, err
	}

	var fs http.FileSystem = http.Dir(abs)

	return FileSystemHandler{impl, key, fs}, nil
}

func CompileBackendHandler(impl *Impl, key string, cfg *TargetConfig) (http.Handler, error) {
	protocol := cfg.Protocol
	address := cfg.Address
	if protocol == "" {
		protocol = "tcp"
	}
	if address == "" {
		return nil, fmt.Errorf("missing required field \"address\"")
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
				return gDialer.DialContext(ctx, protocol, address)
			},
			DisableKeepAlives:  true,
			DisableCompression: true,
			ForceAttemptHTTP2:  true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 3600 * time.Second,
	}

	return BackendHandler{impl, key, protocol, address, client}, nil
}

func toHTTPError(err error) (string, int) {
	switch {
	case os.IsNotExist(err):
		return textNotFound, http.StatusNotFound
	case os.IsPermission(err):
		return textForbidden, http.StatusForbidden
	default:
		return textISE, http.StatusInternalServerError
	}
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

func fileLess(a, b minimalDirEntry) bool {
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

type cacheKey struct {
	dev uint64
	ino uint64
}

type cacheRow struct {
	bytes     []byte
	mtime     time.Time
	md5sum    string
	sha1sum   string
	sha256sum string
	etag      string
}

type minimalDirEntry interface {
	Name() string
	IsDir() bool
}

// type anyDirs {{{

type anyDirs interface {
	sort.Interface
	Name(i int) string
	IsDir(i int) bool
}

// type fileInfoDirs {{{

type fileInfoDirs []fs.FileInfo

func (d fileInfoDirs) Len() int           { return len(d) }
func (d fileInfoDirs) Less(i, j int) bool { return fileLess(d[i], d[j]) }
func (d fileInfoDirs) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d fileInfoDirs) IsDir(i int) bool   { return d[i].IsDir() }
func (d fileInfoDirs) Name(i int) string  { return d[i].Name() }

var _ anyDirs = fileInfoDirs(nil)

// }}}

// type dirEntryDirs {{{

type dirEntryDirs []fs.DirEntry

func (d dirEntryDirs) Len() int           { return len(d) }
func (d dirEntryDirs) Less(i, j int) bool { return fileLess(d[i], d[j]) }
func (d dirEntryDirs) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d dirEntryDirs) IsDir(i int) bool   { return d[i].IsDir() }
func (d dirEntryDirs) Name(i int) string  { return d[i].Name() }

var _ anyDirs = dirEntryDirs(nil)

// }}}

// }}}
