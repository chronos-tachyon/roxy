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
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
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

const defaultMaxCacheSize = 4 << 10 // 4 KiB

var (
	gFileCacheMu  sync.Mutex
	gFileCacheMap map[cacheKey]cacheRow = make(map[cacheKey]cacheRow, 1024)
)

// type BasicSecurityHandler {{{

type BasicSecurityHandler struct {
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

	ctx := r.Context()
	ctx = logger.WithContext(ctx)
	r = r.WithContext(ctx)

	ww := WrapWriter(w)

	logger.Debug().Msg("start request")
	startTime := time.Now()

	defer func() {
		panicValue := recover()
		endTime := time.Now()
		elapsed := endTime.Sub(startTime)

		if panicValue != nil {
			ww.Error(ctx, http.StatusInternalServerError)
		}

		status := ww.Status()
		bytesWritten := ww.BytesWritten()
		sawError := ww.SawError()
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
		if location != "" {
			event = event.Str("location", location)
		}
		if contentType != "" {
			event = event.Str("contentType", contentType)
		}
		event = event.Int64("bytesWritten", bytesWritten)
		event = event.Bool("sawError", sawError)
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
	ctx := r.Context()
	writeError(ctx, w, h.status)
}

var _ http.Handler = ErrorHandler{}

// }}}

// type RedirHandler {{{

type RedirHandler struct {
	tmpl   *template.Template
	status int
}

func (h RedirHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.Ctx(ctx)

	r.URL.Scheme = "https"
	r.URL.Host = r.Host

	var buf strings.Builder
	err := h.tmpl.Execute(&buf, r.URL)
	if err != nil {
		logger.Warn().
			Str("type", "redir").
			Interface("url", r.URL).
			Msg("failed to execute template")
		writeError(ctx, w, http.StatusInternalServerError)
		return
	}

	urlStr := buf.String()
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		logger.Warn().
			Str("type", "redir").
			Str("url", urlStr).
			Msg("invalid url")
		writeError(ctx, w, http.StatusInternalServerError)
		return
	}

	urlStr2 := urlObj.String()
	http.Redirect(w, r, urlStr2, h.status)
}

var _ http.Handler = RedirHandler{}

// }}}

// type FileSystemHandler {{{

type FileSystemHandler struct {
	key string
	fs  http.FileSystem
}

func (h FileSystemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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
		writeError(ctx, w, toHTTPError(err))
		return
	}

	defer reqFile.Close()

	reqStat, err := reqFile.Stat()
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError)
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
				writeError(ctx, w, http.StatusInternalServerError)
				return
			}

			if !idxStat.IsDir() {
				reqPath, reqFile, reqStat = idxPath, idxFile, idxStat
				idxFile, idxStat = nil, nil
			}

		case os.IsNotExist(err):
			// pass

		default:
			writeError(ctx, w, toHTTPError(err))
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
	ctx := r.Context()
	impl := implFromCtx(ctx)
	logger := log.Ctx(ctx)
	hdrs := w.Header()

	var (
		cachePossible bool
		cacheHit      bool
		haveMd5sum    bool
		haveSha1sum   bool
		haveSha256sum bool
		haveEtag      bool
		tookSlowPath  bool
		body          io.ReadSeeker = f
	)

	contentType, contentLang := DetectMimeProperties(impl, logger, h.fs, r.URL.Path)
	hdrs.Set("content-type", contentType)
	if contentLang != "" {
		hdrs.Set("content-language", contentLang)
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
					writeError(ctx, w, http.StatusInternalServerError)
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
					writeError(ctx, w, http.StatusInternalServerError)
					return
				}

				_, err = f.Seek(0, io.SeekStart)
				if err != nil {
					logger.Error().Err(err).Msg("failed to seek to beginning")
					writeError(ctx, w, http.StatusInternalServerError)
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
		Bool("haveMd5sum", haveMd5sum).
		Bool("haveSha1sum", haveSha1sum).
		Bool("haveSha256sum", haveSha256sum).
		Bool("haveEtag", haveEtag).
		Bool("tookSlowPath", tookSlowPath).
		Msg("serve file")
	http.ServeContent(w, r, fi.Name(), mtime, body)
}

func (h FileSystemHandler) ServeDir(w http.ResponseWriter, r *http.Request, f http.File, fi fs.FileInfo) {
	ctx := r.Context()
	impl := implFromCtx(ctx)
	logger := log.Ctx(ctx)
	hdrs := w.Header()

	list, err := f.Readdir(-1)
	if err != nil {
		logger.Error().Err(err).Msg("failed to read directory")
		writeError(ctx, w, http.StatusInternalServerError)
		return
	}

	sort.Sort(fileInfoList(list))

	type entry struct {
		Name        string
		Slash       string
		Mode        string
		Owner       string
		Group       string
		ContentType string
		ContentLang string
		Dev         uint64
		Ino         uint64
		NLink       uint64
		Size        int64
		MTime       time.Time
		IsDir       bool
		IsHidden    bool
	}

	populateRealStats := func(e *entry, fi fs.FileInfo, fullPath string) {
		st, ok := fi.Sys().(*syscall.Stat_t)
		if ok {
			e.Dev = st.Dev
			e.Ino = st.Ino
			e.NLink = st.Nlink

			uid := strconv.FormatUint(uint64(st.Uid), 10)
			if u, err := user.LookupId(uid); err == nil {
				e.Owner = u.Username
			} else {
				logger.Warn().Uint32("uid", st.Uid).Err(err).Msg("failed to look up user by ID")
				e.Owner = "#" + uid
			}

			gid := strconv.FormatUint(uint64(st.Gid), 10)
			if g, err := user.LookupGroupId(gid); err == nil {
				e.Group = g.Name
			} else {
				logger.Warn().Uint32("gid", st.Gid).Err(err).Msg("failed to look up group by ID")
				e.Group = "#" + gid
			}

			var realMode [10]byte
			for i := 0; i < 10; i++ {
				realMode[i] = '-'
			}

			if e.IsDir {
				realMode[0] = 'd'
			}

			if (st.Mode & 0400) == 0400 {
				realMode[1] = 'r'
			}
			if (st.Mode & 0200) == 0200 {
				realMode[2] = 'w'
			}
			switch st.Mode & 04100 {
			case 04100:
				realMode[3] = 's'
			case 04000:
				realMode[3] = 'S'
			case 00100:
				realMode[3] = 'x'
			}

			if (st.Mode & 0040) == 0040 {
				realMode[4] = 'r'
			}
			if (st.Mode & 0020) == 0020 {
				realMode[5] = 'w'
			}
			switch st.Mode & 02010 {
			case 02010:
				realMode[6] = 's'
			case 02000:
				realMode[6] = 'S'
			case 00010:
				realMode[6] = 'x'
			}

			if (st.Mode & 0004) == 0004 {
				realMode[7] = 'r'
			}
			if (st.Mode & 0002) == 0002 {
				realMode[8] = 'w'
			}
			switch st.Mode & 01001 {
			case 01001:
				realMode[9] = 't'
			case 01000:
				realMode[9] = 'T'
			case 00001:
				realMode[9] = 'x'
			}

			e.Mode = string(realMode[:])
		}

		var (
			contentType string
			contentLang string
		)
		if e.IsDir {
			contentType = "inode/directory"
		} else {
			contentType, contentLang = DetectMimeProperties(impl, logger, h.fs, fullPath)
		}

		if contentType == "" {
			contentType = "-"
		}
		if contentLang == "" {
			contentLang = "-"
		}

		e.ContentType = trimContentHeader(contentType)
		e.ContentLang = trimContentHeader(contentLang)
	}

	entries := make([]entry, 0, 1+len(list))
	if r.URL.Path != "/" {
		var e entry

		e.Name = ".."
		e.IsDir = true
		e.Slash = "/"
		e.Mode = "drwxr-xr-x"
		e.Owner = "-"
		e.Group = "-"
		e.IsHidden = false

		parentDir := path.Dir(r.URL.Path)
		if parentFile, err := h.fs.Open(parentDir); err == nil {
			if parentInfo, err := parentFile.Stat(); err == nil {
				populateRealStats(&e, parentInfo, parentDir)
			}
			parentFile.Close()
		}

		entries = append(entries, e)
	}
	for _, fi := range list {
		var e entry

		e.Name = fi.Name()
		e.IsDir = fi.IsDir()
		e.Size = fi.Size()
		e.MTime = fi.ModTime()
		e.Slash = ""
		e.Mode = "-rw-r--r--"
		e.Owner = "owner"
		e.Group = "group"
		e.NLink = 1
		if e.IsDir {
			e.Slash = "/"
			e.Mode = "drwxr-xr-x"
			e.NLink = 0
		}
		e.IsHidden = strings.HasPrefix(e.Name, ".")

		populateRealStats(&e, fi, path.Join(r.URL.Path, e.Name))

		entries = append(entries, e)
	}

	var (
		maxNLinkWidth uint = 1
		maxOwnerWidth uint = 1
		maxGroupWidth uint = 1
		maxSizeWidth  uint = 1
		maxNameWidth  uint = 1
		maxCTypeWidth uint = 1
		maxCLangWidth uint = 1
	)
	for _, e := range entries {
		if w := uint(len(strconv.FormatUint(e.NLink, 10))); w > maxNLinkWidth {
			maxNLinkWidth = w
		}
		if w := uint(len(e.Owner)); w > maxOwnerWidth {
			maxOwnerWidth = w
		}
		if w := uint(len(e.Group)); w > maxGroupWidth {
			maxGroupWidth = w
		}
		if w := uint(len(strconv.FormatInt(e.Size, 10))); w > maxSizeWidth {
			maxSizeWidth = w
		}
		if w := uint(runeLen(e.Name)) + uint(len(e.Slash)); w > maxNameWidth {
			maxNameWidth = w
		}
		if w := uint(len(e.ContentType)); w > maxCTypeWidth {
			maxCTypeWidth = w
		}
		if w := uint(len(e.ContentLang)); w > maxCLangWidth {
			maxCLangWidth = w
		}
	}

	type templateData struct {
		Path             string
		Entries          []entry
		NLinkWidth       uint
		OwnerWidth       uint
		GroupWidth       uint
		SizeWidth        uint
		NameWidth        uint
		ContentTypeWidth uint
		ContentLangWidth uint
	}

	data := templateData{
		Path:             r.URL.Path,
		Entries:          entries,
		NLinkWidth:       maxNLinkWidth,
		OwnerWidth:       maxOwnerWidth,
		GroupWidth:       maxGroupWidth,
		SizeWidth:        maxSizeWidth,
		NameWidth:        maxNameWidth,
		ContentTypeWidth: maxCTypeWidth,
		ContentLangWidth: maxCLangWidth,
	}

	var buf bytes.Buffer
	buf.Grow(4096)

	err = impl.indexPageTmpl.Execute(&buf, data)
	if err != nil {
		logger.Error().Err(err).Msg("failed to render index template")
		writeError(ctx, w, http.StatusInternalServerError)
		return
	}

	raw := buf.Bytes()

	md5raw := md5.Sum(raw)
	sha1raw := sha1.Sum(raw)
	sha256raw := sha256.Sum256(raw)

	md5sum := hex.EncodeToString(md5raw[:])
	sha1sum := hex.EncodeToString(sha1raw[:])
	sha256sum := hex.EncodeToString(sha256raw[:])
	etag := strconv.Quote("D" + sha256sum[:16])

	var (
		contentType string
		contentLang string
	)
	if impl.cfg.IndexPages != nil {
		contentType = impl.cfg.IndexPages.ContentType
		contentLang = impl.cfg.IndexPages.ContentLang
	}
	if contentType == "" {
		contentType = defaultIndexPageType
	}
	if contentLang == "" {
		contentLang = defaultIndexPageLang
	}

	hdrs.Set("content-type", contentType)
	hdrs.Set("content-language", contentLang)
	hdrs.Set("content-md5", md5sum)
	hdrs.Set("content-sha1", sha1sum)
	hdrs.Set("content-sha256", sha256sum)
	hdrs.Set("etag", etag)

	logger.Debug().Msg("serve directory")
	http.ServeContent(w, r, "", fi.ModTime(), bytes.NewReader(raw))
}

var _ http.Handler = FileSystemHandler{}

// }}}

// type BackendHandler {{{

type BackendHandler struct {
	key    string
	proto  string
	addr   string
	client *http.Client
}

func (h BackendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.Ctx(ctx)

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
		writeError(ctx, w, http.StatusInternalServerError)
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

	return FileSystemHandler{key, fs}, nil
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

	return BackendHandler{key, protocol, address, client}, nil
}

func writeError(ctx context.Context, w http.ResponseWriter, statusCode int) {
	if ww, ok := w.(WrappedWriter); ok {
		ww.Error(ctx, statusCode)
	} else {
		statusText := fmt.Sprintf("%03d %s", statusCode, http.StatusText(statusCode))
		http.Error(w, statusText, statusCode)
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
