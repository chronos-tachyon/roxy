package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptrace"
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

	xid "github.com/rs/xid"
	zerolog "github.com/rs/zerolog"
	hlog "github.com/rs/zerolog/hlog"
	log "github.com/rs/zerolog/log"
	unix "golang.org/x/sys/unix"

	"github.com/chronos-tachyon/roxy/internal/balancedclient"
	"github.com/chronos-tachyon/roxy/internal/enums"
)

const defaultMaxCacheSize = 4 << 10 // 4 KiB

const defaultMaxComputeDigestSize = 4 << 20 // 4 MiB

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
	Next http.Handler
}

func (h LoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id := xid.New()

	c := log.Ctx(ctx).With()
	c = c.Str("xid", id.String())
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

	ctx = logger.WithContext(ctx)
	ctx = hlog.CtxWithID(ctx, id)
	r = r.WithContext(ctx)

	r.Header.Set("xid", id.String())
	w.Header().Set("xid", id.String())

	ww := WrapWriter(w)
	ww.SetIsHEAD(r.Method == http.MethodHead)

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
	logger := log.Ctx(ctx)

	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("type", "error").Int("errorStatus", h.status)
	})

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

	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("type", "redir").Int("redirStatus", h.status)
	})

	r.URL.Scheme = "https"
	r.URL.Host = r.Host

	var buf strings.Builder
	err := h.tmpl.Execute(&buf, r.URL)
	if err != nil {
		logger.Warn().
			Interface("arg", r.URL).
			Err(err).
			Msg("failed to execute template")
		writeError(ctx, w, http.StatusInternalServerError)
		return
	}

	urlStr := buf.String()
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		logger.Warn().
			Str("arg", urlStr).
			Err(err).
			Msg("invalid url")
		writeError(ctx, w, http.StatusInternalServerError)
		return
	}

	urlStr2 := urlObj.String()
	writeRedirect(ctx, w, h.status, urlStr2)
}

var _ http.Handler = RedirHandler{}

// }}}

// type FileSystemHandler {{{

type FileSystemHandler struct {
	key string
	fs  http.FileSystem
}

func (h *FileSystemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.Ctx(ctx)

	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("type", "fs").Str("key", h.key)
	})

	if r.Method == http.MethodOptions {
		hdrs := w.Header()
		hdrs.Set("allow", "OPTIONS, GET, HEAD")
		hdrs.Set("cache-control", "max-age=604800")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodHead && r.Method != http.MethodGet {
		hdrs := w.Header()
		hdrs.Set("allow", "OPTIONS, GET, HEAD")
		writeError(ctx, w, http.StatusMethodNotAllowed)
		return
	}

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
		writeRedirect(ctx, w, http.StatusFound, reqPath)
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
		writeRedirect(ctx, w, http.StatusFound, reqPath)
		return
	}

	if !reqStat.IsDir() && strings.HasSuffix(r.URL.Path, "/") {
		writeRedirect(ctx, w, http.StatusFound, reqPath)
		return
	}

	if reqStat.IsDir() {
		idxPath := path.Join(reqPath, "index.html")
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

func (h *FileSystemHandler) ServeFile(w http.ResponseWriter, r *http.Request, f http.File, fi fs.FileInfo) {
	ctx := r.Context()
	impl := implFromCtx(ctx)
	logger := log.Ctx(ctx)
	hdrs := w.Header()

	var (
		cachePossible bool
		cacheHit      bool
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

				md5sum := base64.StdEncoding.EncodeToString(md5raw[:])
				sha1sum := base64.StdEncoding.EncodeToString(sha1raw[:])
				sha256sum := base64.StdEncoding.EncodeToString(sha256raw[:])

				row = cacheRow{
					bytes:     raw,
					mtime:     mtime,
					md5sum:    md5sum,
					sha1sum:   sha1sum,
					sha256sum: sha256sum,
				}

				gFileCacheMu.Lock()
				gFileCacheMap[key] = row
				gFileCacheMu.Unlock()
			}

			setDigestHeader(hdrs, enums.DigestMD5, row.md5sum)
			setDigestHeader(hdrs, enums.DigestSHA1, row.sha1sum)
			setDigestHeader(hdrs, enums.DigestSHA256, row.sha256sum)

			body = bytes.NewReader(row.bytes)
		}
	}

	if !cachePossible {
		var (
			haveMD5    bool
			haveSHA1   bool
			haveSHA256 bool
		)

		if raw, err := readXattr(f, xattrMd5sum); err == nil {
			md5sum := hexToBase64(string(raw))
			setDigestHeader(hdrs, enums.DigestMD5, md5sum)
			haveMD5 = true
		}

		if raw, err := readXattr(f, xattrSha1sum); err == nil {
			sha1sum := hexToBase64(string(raw))
			setDigestHeader(hdrs, enums.DigestSHA1, sha1sum)
			haveSHA1 = true
		}

		if raw, err := readXattr(f, xattrSha256sum); err == nil {
			sha256sum := hexToBase64(string(raw))
			setDigestHeader(hdrs, enums.DigestSHA256, sha256sum)
			haveSHA256 = true
		}

		if raw, err := readXattr(f, xattrEtag); err == nil {
			etag := string(raw)
			hdrs.Set("etag", etag)
		}

		if !haveMD5 && !haveSHA1 && !haveSHA256 && fi.Size() <= defaultMaxComputeDigestSize {
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

				md5sum := base64.StdEncoding.EncodeToString(md5writer.Sum(nil))
				sha1sum := base64.StdEncoding.EncodeToString(sha1writer.Sum(nil))
				sha256sum := base64.StdEncoding.EncodeToString(sha256writer.Sum(nil))

				setDigestHeader(hdrs, enums.DigestMD5, md5sum)
				setDigestHeader(hdrs, enums.DigestSHA1, sha1sum)
				setDigestHeader(hdrs, enums.DigestSHA256, sha256sum)
				tookSlowPath = true
			}
		}

	}

	setETagHeader(hdrs, "", fi.ModTime())

	logger.Debug().
		Bool("cachePossible", cachePossible).
		Bool("cacheHit", cacheHit).
		Bool("tookSlowPath", tookSlowPath).
		Msg("serve file")
	http.ServeContent(w, r, fi.Name(), mtime, body)
}

func (h *FileSystemHandler) ServeDir(w http.ResponseWriter, r *http.Request, f http.File, fi fs.FileInfo) {
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
		Link        string
		ContentType string
		ContentLang string
		Dev         uint64
		Ino         uint64
		NLink       uint64
		Size        int64
		MTime       time.Time
		IsDir       bool
		IsLink      bool
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
			if (st.Mode & syscall.S_IFMT) == syscall.S_IFLNK {
				realMode[0] = 'l'
				e.IsLink = true
				if link, err := readLinkAt(f, fi.Name()); err == nil {
					e.Link = link
				}
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
		} else if !e.IsLink {
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
		maxLinkWidth  uint = 1
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
		if w := uint(runeLen(e.Link)); w > maxLinkWidth {
			maxLinkWidth = w
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
		LinkWidth        uint
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
		LinkWidth:        maxLinkWidth,
		ContentTypeWidth: maxCTypeWidth,
		ContentLangWidth: maxCLangWidth,
	}

	page := impl.pages["index"]

	var buf bytes.Buffer
	buf.Grow(page.size)
	err = page.tmpl.Execute(&buf, data)
	if err != nil {
		logger.Error().Err(err).Msg("failed to render index template")
		writeError(ctx, w, http.StatusInternalServerError)
		return
	}

	raw := buf.Bytes()

	md5raw := md5.Sum(raw)
	sha1raw := sha1.Sum(raw)
	sha256raw := sha256.Sum256(raw)

	md5sum := base64.StdEncoding.EncodeToString(md5raw[:])
	sha1sum := base64.StdEncoding.EncodeToString(sha1raw[:])
	sha256sum := base64.StdEncoding.EncodeToString(sha256raw[:])

	hdrs.Set("content-type", page.contentType)
	hdrs.Set("content-language", page.contentLang)
	setDigestHeader(hdrs, enums.DigestMD5, md5sum)
	setDigestHeader(hdrs, enums.DigestSHA1, sha1sum)
	setDigestHeader(hdrs, enums.DigestSHA256, sha256sum)
	setETagHeader(hdrs, "D", fi.ModTime())

	logger.Debug().Msg("serve directory")
	http.ServeContent(w, r, "", fi.ModTime(), bytes.NewReader(raw))
}

var _ http.Handler = (*FileSystemHandler)(nil)

// }}}

// type HTTPBackendHandler {{{

type HTTPBackendHandler struct {
	key    string
	cfg    *TargetConfig
	client *balancedclient.BalancedClient
}

func (h *HTTPBackendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.Ctx(ctx)
	laddr := laddrFromCtx(ctx)
	raddr := raddrFromCtx(ctx)

	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("type", "http").Str("key", h.key).Interface("target", h.cfg)
	})

	var once sync.Once
	var addr net.Addr
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			once.Do(func() {
				addr = info.Conn.RemoteAddr()
			})
		},
	})

	resType := h.cfg.Resolver
	if resType == enums.DefaultResolver {
		resType = enums.DNSResolver
	}
	r.Header.Set("roxy-target", fmt.Sprintf("%s; %s; %s", h.key, resType, h.cfg.Address))

	innerURL := new(url.URL)
	*innerURL = *r.URL
	innerURL.Scheme = "http"
	innerURL.Host = h.client.Resolver().ServerHostname()
	if h.client.IsTLS() {
		innerURL.Scheme = "https"
	}

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

	innerReq.Close = true
	innerReq.Host = r.Host
	innerReq.Header.Set("host", r.Host)
	innerReq.Header.Set("x-forwarded-host", r.Host)
	innerReq.Header.Set("x-forwarded-proto", "https")
	innerReq.Header.Set("x-forwarded-ip", raddr.String())
	innerReq.Header.Add("x-forwarded-for", raddr.String())
	innerReq.Header.Add("forwarded", fmt.Sprintf("by=%s;for=%s;host=%s;proto=%s", laddr, raddr, r.Host, "https"))

	if values := innerReq.Header.Values("x-forwarded-for"); len(values) > 1 {
		innerReq.Header.Set("x-forwarded-for", strings.Join(values, ", "))
	}

	innerResp, err := h.client.Do(innerReq)
	once.Do(func() {})
	if addr != nil {
		logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("backend", addr.String())
		})
	}
	if err != nil {
		logger.Error().
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
			Err(err).
			Msg("failed subrequest")
	}

	needBodyClose = false
	err = innerResp.Body.Close()
	if err != nil {
		logger.Warn().
			Err(err).
			Msg("failed to close body of subrequest")
	}
}

func (h *HTTPBackendHandler) Close() error {
	return h.client.Close()
}

var _ http.Handler = (*HTTPBackendHandler)(nil)

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
	case enums.UndefinedTargetType:
		return nil, fmt.Errorf("missing required field \"type\"")

	case enums.FileSystemTargetType:
		return CompileFileSystemHandler(impl, key, cfg)

	case enums.HTTPBackendTargetType:
		return CompileHTTPBackendHandler(impl, key, cfg)

	default:
		panic(fmt.Errorf("unknown target type %#v", cfg.Type))
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

	return &FileSystemHandler{key, fs}, nil
}

func CompileHTTPBackendHandler(impl *Impl, key string, cfg *TargetConfig) (http.Handler, error) {
	tlsConfig, err := CompileTLSClientConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}

	bc, err := balancedclient.New(cfg.Resolver, balancedclient.Options{
		Context:      gRootContext,
		Target:       cfg.Address,
		Balancer:     cfg.Balancer,
		PollInterval: cfg.PollInterval,
		Etcd:         impl.etcd,
		ZK:           impl.zk,
		Dialer:       &gDialer,
		TLSConfig:    tlsConfig,
	})
	if err != nil {
		return nil, err
	}
	return &HTTPBackendHandler{key, cfg, bc}, nil
}

func writeRedirect(ctx context.Context, w http.ResponseWriter, statusCode int, urlstr string) {
	ww := w.(WrappedWriter)
	ww.Redirect(ctx, statusCode, urlstr)
}

func writeError(ctx context.Context, w http.ResponseWriter, statusCode int) {
	ww := w.(WrappedWriter)
	ww.Error(ctx, statusCode)
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
