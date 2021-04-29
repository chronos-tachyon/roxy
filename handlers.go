package main

import (
	"bytes"
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/common/baseresolver"
	_ "github.com/chronos-tachyon/roxy/grpc/balancer/atcbalancer"
	"github.com/chronos-tachyon/roxy/grpc/resolver/atcresolver"
	"github.com/chronos-tachyon/roxy/grpc/resolver/etcdresolver"
	"github.com/chronos-tachyon/roxy/grpc/resolver/zkresolver"
	"github.com/chronos-tachyon/roxy/internal/balancedclient"
	"github.com/chronos-tachyon/roxy/internal/enums"
	"github.com/chronos-tachyon/roxy/roxypb"
)

const defaultMaxCacheSize = 4 << 10 // 4 KiB

const defaultMaxComputeDigestSize = 4 << 20 // 4 MiB

var (
	gFileCacheMu  sync.Mutex
	gFileCacheMap map[cacheKey]cacheRow = make(map[cacheKey]cacheRow, 1024)
)

var gMetrics = map[string]*Metrics{
	"prom":  NewMetrics("prom"),
	"http":  NewMetrics("http"),
	"https": NewMetrics("https"),
}

func init() {
	for _, metrics := range gMetrics {
		metrics.MustRegister(prometheus.DefaultRegisterer)
	}
}

// type Metrics {{{

type Metrics struct {
	PanicCount                   prometheus.Counter
	RequestCountByMethod         *prometheus.CounterVec
	ResponseCountByCode          *prometheus.CounterVec
	RequestSize                  prometheus.Histogram
	ResponseSize                 prometheus.Histogram
	ResponseDuration             prometheus.Histogram
	ResponseCountByCodeAndTarget *prometheus.CounterVec
	ProblemCountByTypeAndTarget  *prometheus.CounterVec
	ResponseSizeByTarget         *prometheus.HistogramVec
	ResponseDurationByTarget     *prometheus.HistogramVec
}

func NewMetrics(proto string) *Metrics {
	m := new(Metrics)

	m.PanicCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "roxy",
			Subsystem: proto,
			Name:      "panic_count",
			Help:      "the number of HTTP requests whose handlers paniced",
		},
	)

	m.RequestCountByMethod = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "roxy",
			Subsystem: proto,
			Name:      "request_by_method_count",
			Help:      "the number of incoming HTTP requests",
		},
		[]string{"method"},
	)

	m.ResponseCountByCode = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "roxy",
			Subsystem: proto,
			Name:      "response_by_code_count",
			Help:      "the number of outgoing HTTP responses",
		},
		[]string{"code"},
	)

	m.RequestSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "roxy",
			Subsystem: proto,
			Name:      "request_size_bytes",
			Help:      "the size of the incoming HTTP request",
			Buckets:   prometheus.ExponentialBuckets(1024, 4, 10),
		},
	)

	m.ResponseSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "roxy",
			Subsystem: proto,
			Name:      "response_size_bytes",
			Help:      "the size of the outgoing HTTP response",
			Buckets:   prometheus.ExponentialBuckets(1024, 4, 10),
		},
	)

	m.ResponseDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "roxy",
			Subsystem: proto,
			Name:      "response_duration_secs",
			Help:      "the duration of the HTTP request lifetime",
			Buckets:   prometheus.ExponentialBuckets(0.001, 10, 7),
		},
	)

	if proto == "https" {
		m.ResponseCountByCodeAndTarget = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "roxy",
				Subsystem: proto,
				Name:      "response_by_code_and_target_count",
				Help:      "the number of outgoing HTTP responses",
			},
			[]string{"code", "target"},
		)

		m.ProblemCountByTypeAndTarget = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "roxy",
				Subsystem: proto,
				Name:      "problem_by_type_and_target_count",
				Help:      "the number of failed HTTP responses",
			},
			[]string{"type", "target"},
		)

		m.ResponseSizeByTarget = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "roxy",
				Subsystem: proto,
				Name:      "response_size_by_target_bytes",
				Help:      "the size of the outgoing HTTP response",
				Buckets:   prometheus.ExponentialBuckets(1024, 4, 10),
			},
			[]string{"target"},
		)

		m.ResponseDurationByTarget = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "roxy",
				Subsystem: proto,
				Name:      "response_duration_by_target_secs",
				Help:      "the duration of the HTTP request lifetime",
				Buckets:   prometheus.ExponentialBuckets(0.001, 10, 7),
			},
			[]string{"target"},
		)
	}

	for _, method := range []string{
		http.MethodOptions,
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	} {
		m.RequestCountByMethod.WithLabelValues(simplifyHTTPMethod(method))
	}

	for _, code := range []int{
		http.StatusOK,
		http.StatusNoContent,
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusNotModified,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	} {
		m.ResponseCountByCode.WithLabelValues(simplifyHTTPStatusCode(code))
	}

	return m
}

func (m *Metrics) All() []prometheus.Collector {
	out := make([]prometheus.Collector, 0, 10)
	out = append(
		out,
		m.PanicCount,
		m.RequestCountByMethod,
		m.ResponseCountByCode,
		m.RequestSize,
		m.ResponseSize,
		m.ResponseDuration,
	)
	if m.ResponseCountByCodeAndTarget != nil {
		out = append(
			out,
			m.ResponseCountByCodeAndTarget,
			m.ProblemCountByTypeAndTarget,
			m.ResponseSizeByTarget,
			m.ResponseDurationByTarget,
		)
	}
	return out
}

func (m *Metrics) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(m.All()...)
}

// }}}

// type RootHandler {{{

type RootHandler struct {
	Ref  *Ref
	Next http.Handler
}

func (h RootHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := xid.New()
	idStr := id.String()
	r.Header.Set("xid", idStr)
	w.Header().Set("xid", idStr)
	w.Header().Set("server", "roxy/"+Version())
	w.Header().Set("content-security-policy", "default-src 'self';")
	w.Header().Set("strict-transport-security", "max-age=86400")
	w.Header().Set("x-content-type-options", "nosniff")
	w.Header().Set("x-xss-protection", "1; mode=block")

	ctx := r.Context()
	cc := GetConnContext(ctx)
	impl := h.Ref.Get()

	c := cc.Logger.With()
	c = c.Str("xid", idStr)
	c = c.Str("ip", addrWithNoPort(cc.RemoteAddr))
	c = c.Str("method", r.Method)
	c = c.Str("host", r.Host)
	c = c.Str("url", r.URL.String())
	if value := r.Header.Get("user-agent"); value != "" {
		c = c.Str("userAgent", value)
	}
	if value := r.Header.Get("referer"); value != "" {
		c = c.Str("referer", value)
	}

	rc := &RequestContext{
		Logger:     c.Logger(),
		Proto:      cc.Proto,
		LocalAddr:  cc.LocalAddr,
		RemoteAddr: cc.RemoteAddr,
		XID:        id,
		Impl:       impl,
		Metrics:    gMetrics[cc.Proto],
		Body:       WrapReader(r.Body),
		Writer:     WrapWriter(impl, w, r),
	}
	ctx = WithRequestContext(ctx, rc)
	ctx = rc.Logger.WithContext(ctx)
	ctx = hlog.CtxWithID(ctx, id)
	r = r.WithContext(ctx)
	rc.Request = r
	rc.Context = ctx

	rc.Logger.Debug().
		Msg("start request")
	rc.StartTime = time.Now()
	rc.Metrics.RequestCountByMethod.WithLabelValues(simplifyHTTPMethod(r.Method)).Inc()

	defer func() {
		if !rc.Body.WasClosed() {
			io.Copy(io.Discard, rc.Body)
			rc.Body.Close()
		}

		rc.EndTime = time.Now()

		panicValue := recover()
		if panicValue != nil {
			rc.Writer.WriteError(http.StatusInternalServerError)
			rc.Metrics.PanicCount.Inc()
		}

		elapsed := rc.EndTime.Sub(rc.StartTime)
		duration := float64(elapsed) / float64(time.Second)
		bytesRead := float64(rc.Body.BytesRead())
		bytesWritten := float64(rc.Writer.BytesWritten())
		code := simplifyHTTPStatusCode(rc.Writer.Status())

		var problemType string
		switch {
		case rc.Writer.SawError():
			problemType = "error"
		case rc.Writer.Status() >= 500:
			problemType = "5xx"
		case rc.Writer.Status() >= 400:
			problemType = "4xx"
		default:
			problemType = "none"
		}

		rc.Metrics.ResponseCountByCode.WithLabelValues(code).Inc()
		rc.Metrics.RequestSize.Observe(bytesRead)
		rc.Metrics.ResponseSize.Observe(bytesWritten)
		rc.Metrics.ResponseDuration.Observe(duration)

		if rc.Metrics.ResponseCountByCodeAndTarget != nil {
			rc.Metrics.ResponseCountByCodeAndTarget.WithLabelValues(code, rc.TargetKey).Inc()
			rc.Metrics.ProblemCountByTypeAndTarget.WithLabelValues(problemType, rc.TargetKey).Inc()
			rc.Metrics.ResponseSizeByTarget.WithLabelValues(rc.TargetKey).Observe(bytesWritten)
			rc.Metrics.ResponseDurationByTarget.WithLabelValues(rc.TargetKey).Observe(duration)
		}

		var event *zerolog.Event
		if panicValue != nil {
			event = rc.Logger.Error()
			switch x := panicValue.(type) {
			case error:
				event = event.AnErr("panic", x)
			case string:
				event = event.Str("panic", x)
			default:
				event = event.Interface("panic", x)
			}
		} else {
			event = rc.Logger.Info()
		}
		event = event.Dur("elapsed", elapsed)
		event = event.Int("status", rc.Writer.Status())
		if location := rc.Writer.Header().Get("location"); location != "" {
			event = event.Str("location", location)
		}
		if contentType := rc.Writer.Header().Get("content-type"); contentType != "" {
			event = event.Str("contentType", contentType)
		}
		event = event.Int64("bytesRead", rc.Body.BytesRead())
		event = event.Int64("bytesWritten", rc.Writer.BytesWritten())
		event = event.Bool("sawError", rc.Writer.SawError())
		event.Msg("end request")
	}()

	h.Next.ServeHTTP(rc.Writer, rc.Request)
}

var _ http.Handler = RootHandler{}

// }}}

// type InsecureHandler {{{

type InsecureHandler struct {
	Next         http.Handler
	mu           sync.Mutex
	savedImpl    *Impl
	savedHandler http.Handler
}

func (h *InsecureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var next http.Handler
	rc := GetRequestContext(r.Context())
	rc.TargetKey = "!HTTP"

	h.mu.Lock()
	if rc.Impl == h.savedImpl {
		next = h.savedHandler
	} else {
		next = rc.Impl.ACMEManager().HTTPHandler(h.Next)
		h.savedImpl = rc.Impl
		h.savedHandler = next
	}
	h.mu.Unlock()

	next.ServeHTTP(w, r)
}

var _ http.Handler = (*InsecureHandler)(nil)

// }}}

// type SecureHandler {{{

type SecureHandler struct{}

func (SecureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := GetRequestContext(ctx)

	applicableRules := make([]*Rule, 0, len(rc.Impl.rules))
	for _, rule := range rc.Impl.rules {
		if rule.Check(rc.Request) {
			applicableRules = append(applicableRules, rule)
			if rule.IsTerminal() {
				break
			}
		}
	}

	for _, rule := range applicableRules {
		rule.ApplyFirst(rc.Writer, rc.Request)
	}

	for _, rule := range applicableRules {
		rule.ApplyPre(rc.Writer, rc.Request)
	}

	rc.Writer.SetRules(applicableRules)

	if len(applicableRules) != 0 {
		lastIndex := len(applicableRules) - 1
		lastRule := applicableRules[lastIndex]
		if lastRule.IsTerminal() {
			rc.TargetKey = lastRule.TargetKey
			rc.TargetConfig = lastRule.TargetConfig
			rc.Logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
				c = c.Str("key", simplifyTargetKey(lastRule.TargetKey))
				if lastRule.TargetConfig != nil {
					c = c.Interface("target", lastRule.TargetConfig)
				}
				return c
			})
			lastRule.TargetHandler.ServeHTTP(w, r)
			return
		}
	}

	rc.TargetKey = "!EOF"
	rc.Writer.WriteError(http.StatusNotFound)
	rc.Request.URL.Scheme = "https"
	rc.Request.URL.Host = rc.Request.Host
	rc.Logger.Warn().
		Str("mutatedURL", rc.Request.URL.String()).
		Msg("no matching target")
}

var _ http.Handler = SecureHandler{}

// }}}

// type ErrorHandler {{{

type ErrorHandler struct {
	Status int
}

func (h ErrorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := GetRequestContext(r.Context())
	rc.Writer.WriteError(h.Status)
}

var _ http.Handler = ErrorHandler{}

// }}}

// type RedirHandler {{{

type RedirHandler struct {
	Template *template.Template
	Status   int
}

func (h RedirHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := GetRequestContext(r.Context())

	u := new(url.URL)
	*u = *rc.Request.URL
	u.Scheme = "https"
	u.Host = rc.Request.Host

	var buf strings.Builder
	err := h.Template.Execute(&buf, u)
	if err != nil {
		rc.Logger.Warn().
			Interface("arg", u).
			Err(err).
			Msg("failed to execute template")
		rc.Writer.WriteError(http.StatusInternalServerError)
		return
	}
	str := buf.String()

	u2, err := url.Parse(str)
	if err != nil {
		rc.Logger.Warn().
			Str("arg", str).
			Err(err).
			Msg("invalid url")
		rc.Writer.WriteError(http.StatusInternalServerError)
		return
	}

	str = u2.String()
	rc.Writer.WriteRedirect(h.Status, str)
}

var _ http.Handler = RedirHandler{}

// }}}

// type FileSystemHandler {{{

type FileSystemHandler struct {
	fs http.FileSystem
}

func (h *FileSystemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := GetRequestContext(r.Context())

	hdrs := rc.Writer.Header()
	hdrs.Set("cache-control", "public, max-age=86400, must-revalidate")

	if rc.Request.Method == http.MethodOptions {
		hdrs.Set("allow", "OPTIONS, GET, HEAD")
		rc.Writer.WriteHeader(http.StatusNoContent)
		return
	}

	if rc.Request.Method != http.MethodHead && rc.Request.Method != http.MethodGet {
		hdrs.Set("allow", "OPTIONS, GET, HEAD")
		hdrs.Del("cache-control")
		rc.Writer.WriteError(http.StatusMethodNotAllowed)
		return
	}

	reqPath := rc.Request.URL.Path
	if !strings.HasPrefix(reqPath, "/") {
		reqPath = "/" + reqPath
	}
	reqPath = path.Clean(reqPath)

	if path.Base(reqPath) == "index.html" {
		reqPath = path.Dir(reqPath)
		if !strings.HasSuffix(reqPath, "/") {
			reqPath += "/"
		}
		rc.Writer.WriteRedirect(http.StatusFound, reqPath)
		return
	}

	reqFile, err := h.fs.Open(reqPath)
	if err != nil {
		rc.Writer.WriteError(toHTTPError(err))
		return
	}

	defer reqFile.Close()

	reqStat, err := reqFile.Stat()
	if err != nil {
		rc.Writer.WriteError(http.StatusInternalServerError)
		return
	}

	if reqStat.IsDir() && !strings.HasSuffix(rc.Request.URL.Path, "/") {
		reqPath += "/"
		rc.Writer.WriteRedirect(http.StatusFound, reqPath)
		return
	}

	if !reqStat.IsDir() && strings.HasSuffix(rc.Request.URL.Path, "/") {
		rc.Writer.WriteRedirect(http.StatusFound, reqPath)
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
				rc.Writer.WriteError(http.StatusInternalServerError)
				return
			}

			if !idxStat.IsDir() {
				reqPath, reqFile, reqStat = idxPath, idxFile, idxStat
				idxFile, idxStat = nil, nil
			}

		case os.IsNotExist(err):
			// pass

		default:
			rc.Writer.WriteError(toHTTPError(err))
			return
		}
	}

	rc.Request.URL.Path = reqPath

	if reqStat.IsDir() {
		h.ServeDir(rc, reqFile, reqStat)
		return
	}

	h.ServeFile(rc, reqFile, reqStat)
}

func (h *FileSystemHandler) ServeFile(rc *RequestContext, f http.File, fi fs.FileInfo) {
	hdrs := rc.Writer.Header()

	var (
		cachePossible bool
		cacheHit      bool
		tookSlowPath  bool
		body          io.ReadSeeker = f
	)

	contentType, contentLang, contentEnc := DetectMimeProperties(rc.Impl, rc.Logger, h.fs, rc.Request.URL.Path)
	hdrs.Set("content-type", contentType)
	if contentLang != "" {
		hdrs.Set("content-language", contentLang)
	}
	if contentEnc != "" {
		hdrs.Set("content-encoding", contentEnc)
	}

	if contentType == "application/javascript" || contentType == "text/javascript" || strings.HasPrefix(contentType, "text/javascript;") {
		mapFilePath := rc.Request.URL.Path + ".map"
		mapFile, err := h.fs.Open(mapFilePath)
		if err == nil {
			mapFile.Close()
			hdrs.Set("sourcemap", mapFilePath)
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
					rc.Writer.WriteError(http.StatusInternalServerError)
					rc.Logger.Error().
						Err(err).
						Msg("failed to read file")
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
					rc.Writer.WriteError(http.StatusInternalServerError)
					rc.Logger.Error().
						Err(err).
						Msg("failed to read file contents")
					return
				}

				_, err = f.Seek(0, io.SeekStart)
				if err != nil {
					rc.Writer.WriteError(http.StatusInternalServerError)
					rc.Logger.Error().
						Err(err).
						Msg("failed to seek to beginning")
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

	rc.Logger.Debug().
		Bool("cachePossible", cachePossible).
		Bool("cacheHit", cacheHit).
		Bool("tookSlowPath", tookSlowPath).
		Msg("serve file")
	http.ServeContent(rc.Writer, rc.Request, fi.Name(), mtime, body)
}

func (h *FileSystemHandler) ServeDir(rc *RequestContext, f http.File, fi fs.FileInfo) {
	hdrs := rc.Writer.Header()

	list, err := f.Readdir(-1)
	if err != nil {
		rc.Writer.WriteError(http.StatusInternalServerError)
		rc.Logger.Error().
			Err(err).
			Msg("failed to read directory")
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
		ContentEnc  string
		Dev         uint64
		Ino         uint64
		NLink       uint32
		Size        int64
		MTime       time.Time
		IsDir       bool
		IsLink      bool
		IsHidden    bool
	}

	uidCache := make(map[uint32]string, 4)
	gidCache := make(map[uint32]string, 4)

	lookupUID := func(uid uint32) string {
		if name, found := uidCache[uid]; found {
			return name
		}

		str := strconv.FormatUint(uint64(uid), 10)
		u, err := user.LookupId(str)
		var name string
		if err == nil {
			name = u.Username
		} else {
			rc.Logger.Warn().
				Uint32("uid", uid).
				Err(err).
				Msg("failed to look up user by ID")
			name = "#" + str
		}
		uidCache[uid] = name
		return name
	}

	lookupGID := func(gid uint32) string {
		if name, found := gidCache[gid]; found {
			return name
		}

		str := strconv.FormatUint(uint64(gid), 10)
		g, err := user.LookupGroupId(str)
		var name string
		if err == nil {
			name = g.Name
		} else {
			rc.Logger.Warn().
				Uint32("gid", gid).
				Err(err).
				Msg("failed to look up group by ID")
			name = "#" + str
		}
		gidCache[gid] = name
		return name
	}

	populateRealStats := func(e *entry, fi fs.FileInfo, fullPath string) {
		st, ok := fi.Sys().(*syscall.Stat_t)
		if ok {
			e.Dev = st.Dev
			e.Ino = st.Ino
			e.NLink = uint32(st.Nlink)
			e.Owner = lookupUID(st.Uid)
			e.Group = lookupGID(st.Gid)

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
			contentEnc  string
		)
		if e.IsDir {
			contentType = "inode/directory"
		} else if !e.IsLink {
			contentType, contentLang, contentEnc = DetectMimeProperties(rc.Impl, rc.Logger, h.fs, fullPath)
		}

		if contentType == "" {
			contentType = "-"
		}
		if contentLang == "" {
			contentLang = "-"
		}
		if contentEnc == "" {
			contentEnc = "-"
		}

		e.ContentType = trimContentHeader(contentType)
		e.ContentLang = trimContentHeader(contentLang)
		e.ContentEnc = trimContentHeader(contentEnc)
	}

	entries := make([]entry, 0, 1+len(list))
	if rc.Request.URL.Path != "/" {
		var e entry

		e.Name = ".."
		e.IsDir = true
		e.Slash = "/"
		e.Mode = "drwxr-xr-x"
		e.Owner = "-"
		e.Group = "-"
		e.IsHidden = false

		parentDir := path.Dir(rc.Request.URL.Path)
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

		populateRealStats(&e, fi, path.Join(rc.Request.URL.Path, e.Name))

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
		maxCEncWidth  uint = 1
	)
	for _, e := range entries {
		if w := uint(len(strconv.FormatUint(uint64(e.NLink), 10))); w > maxNLinkWidth {
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
		if w := uint(len(e.ContentEnc)); w > maxCEncWidth {
			maxCEncWidth = w
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
		ContentEncWidth  uint
	}

	data := templateData{
		Path:             rc.Request.URL.Path,
		Entries:          entries,
		NLinkWidth:       maxNLinkWidth,
		OwnerWidth:       maxOwnerWidth,
		GroupWidth:       maxGroupWidth,
		SizeWidth:        maxSizeWidth,
		NameWidth:        maxNameWidth,
		LinkWidth:        maxLinkWidth,
		ContentTypeWidth: maxCTypeWidth,
		ContentLangWidth: maxCLangWidth,
		ContentEncWidth:  maxCEncWidth,
	}

	page := rc.Impl.pages["index"]

	var buf bytes.Buffer
	buf.Grow(page.size)
	err = page.tmpl.Execute(&buf, data)
	if err != nil {
		rc.Writer.WriteError(http.StatusInternalServerError)
		rc.Logger.Error().
			Err(err).
			Msg("failed to render index template")
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
	if page.contentLang != "" {
		hdrs.Set("content-language", page.contentLang)
	}
	if page.contentEnc != "" {
		hdrs.Set("content-encoding", page.contentEnc)
	}
	setDigestHeader(hdrs, enums.DigestMD5, md5sum)
	setDigestHeader(hdrs, enums.DigestSHA1, sha1sum)
	setDigestHeader(hdrs, enums.DigestSHA256, sha256sum)
	setETagHeader(hdrs, "D", fi.ModTime())

	rc.Logger.Debug().
		Msg("serve directory")
	http.ServeContent(rc.Writer, rc.Request, "", fi.ModTime(), bytes.NewReader(raw))
}

var _ http.Handler = (*FileSystemHandler)(nil)

// }}}

// type HTTPBackendHandler {{{

type HTTPBackendHandler struct {
	client *balancedclient.BalancedClient
}

func (h *HTTPBackendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := GetRequestContext(ctx)

	var once sync.Once
	var addr net.Addr
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			once.Do(func() {
				addr = info.Conn.RemoteAddr()
			})
		},
	})

	rc.Request.Header.Set("roxy-target", rc.RoxyTarget())
	rc.Request.Header.Set("x-forwarded-host", rc.Request.Host)
	rc.Request.Header.Set("x-forwarded-proto", "https")
	rc.Request.Header.Set("x-forwarded-ip", addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add("x-forwarded-for", addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add(
		"forwarded",
		fmt.Sprintf("by=%s;for=%s;host=%s;proto=%s",
			quoteForwardedAddr(rc.LocalAddr),
			quoteForwardedAddr(rc.RemoteAddr),
			rc.Request.Host,
			"https"))

	if values := rc.Request.Header.Values("x-forwarded-for"); len(values) > 1 {
		rc.Request.Header.Set("x-forwarded-for", strings.Join(values, ", "))
	}

	innerURL := new(url.URL)
	*innerURL = *rc.Request.URL
	innerURL.Scheme = "http"
	innerURL.Host = "example.com"
	if h.client.IsTLS() {
		innerURL.Scheme = "https"
	}

	innerReq, err := http.NewRequestWithContext(
		ctx,
		rc.Request.Method,
		innerURL.String(),
		rc.Body)
	if err != nil {
		panic(err)
	}

	innerReq.Header = make(http.Header, len(rc.Request.Header))
	for key, oldValues := range rc.Request.Header {
		newValues := make([]string, len(oldValues))
		copy(newValues, oldValues)
		innerReq.Header[key] = newValues
	}

	innerReq.Close = true
	innerReq.Host = rc.Request.Host
	innerReq.Header.Set("host", rc.Request.Host)

	innerResp, err := h.client.Do(innerReq)
	once.Do(func() {})
	if addr != nil {
		rc.Logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("backend", addr.String())
		})
	}
	if err != nil {
		rc.Writer.WriteError(http.StatusInternalServerError)
		rc.Logger.Error().
			Err(err).
			Msg("failed subrequest")
		return
	}

	needBodyClose := true
	defer func() {
		if needBodyClose {
			innerResp.Body.Close()
		}
	}()

	hdrs := rc.Writer.Header()
	for key, oldValues := range innerResp.Header {
		newValues := make([]string, len(oldValues))
		copy(newValues, oldValues)
		hdrs[key] = newValues
	}
	rc.Writer.WriteHeader(innerResp.StatusCode)

	_, err = io.Copy(rc.Writer, innerResp.Body)
	if err != nil {
		rc.Logger.Error().
			Err(err).
			Msg("failed subrequest")
	}

	needBodyClose = false
	err = innerResp.Body.Close()
	if err != nil {
		rc.Logger.Warn().
			Err(err).
			Msg("failed to close body of subrequest")
	}
}

func (h *HTTPBackendHandler) Close() error {
	return h.client.Close()
}

var _ http.Handler = (*HTTPBackendHandler)(nil)

// }}}

// type GRPCBackendHandler {{{

type GRPCBackendHandler struct {
	cc *grpc.ClientConn
}

func (h *GRPCBackendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := GetRequestContext(ctx)

	rc.Request.Header.Set("roxy-target", rc.RoxyTarget())
	rc.Request.Header.Set("x-forwarded-host", rc.Request.Host)
	rc.Request.Header.Set("x-forwarded-proto", "https")
	rc.Request.Header.Set("x-forwarded-ip", addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add("x-forwarded-for", addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add(
		"forwarded",
		fmt.Sprintf("by=%s;for=%s;host=%s;proto=%s",
			quoteForwardedAddr(rc.LocalAddr),
			quoteForwardedAddr(rc.RemoteAddr),
			rc.Request.Host,
			"https"))

	var wg sync.WaitGroup
	needBodyClose := true
	defer func() {
		if needBodyClose {
			io.Copy(io.Discard, rc.Body)
			rc.Body.Close()
		}
		wg.Wait()
	}()

	webclient := roxypb.NewWebServerClient(h.cc)
	sc, err := webclient.Serve(ctx)
	if err != nil {
		rc.Writer.WriteError(http.StatusServiceUnavailable)
		rc.Logger.Error().
			Err(err).
			Msg("failed to call /roxy.WebServer/Serve")
		return
	}

	needCloseSend := true
	defer func() {
		if needCloseSend {
			sc.CloseSend()
		}
	}()

	wg.Add(1)
	go h.recvThread(rc, sc, &wg)

	kv := make([]*roxypb.KeyValue, 4, 4+len(rc.Request.Header))
	kv[0] = &roxypb.KeyValue{Key: ":scheme", Value: "https"}
	kv[1] = &roxypb.KeyValue{Key: ":method", Value: rc.Request.Method}
	kv[2] = &roxypb.KeyValue{Key: ":authority", Value: rc.Request.Host}
	kv[3] = &roxypb.KeyValue{Key: ":path", Value: rc.Request.URL.Path}

	keys := make(headerNames, 0, len(rc.Request.Header))
	for key := range rc.Request.Header {
		keys = append(keys, key)
	}
	keys.Sort()
	for _, key := range keys {
		lc := strings.ToLower(key)
		for _, value := range rc.Request.Header[key] {
			kv = append(kv, &roxypb.KeyValue{Key: lc, Value: value})
		}
	}

	err = sc.Send(&roxypb.WebMessage{Headers: kv})
	if err != nil {
		rc.Logger.Error().
			Err(err).
			Msg("/roxy.WebServer/Serve: Send failed")
		return
	}

	buf := make([]byte, 1<<20) // 1 MiB
	for {
		n, err := io.ReadFull(rc.Body, buf)
		if n > 0 {
			err2 := sc.Send(&roxypb.WebMessage{BodyChunk: buf[:n]})
			if err2 != nil {
				rc.Logger.Error().
					Err(err).
					Msg("/roxy.WebServer/Serve: Send failed")
				return
			}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			rc.Logger.Error().
				Err(err).
				Msg("failed to Read request body")
			return
		}
	}

	needBodyClose = false
	err = rc.Body.Close()
	if err != nil {
		rc.Logger.Warn().
			Err(err).
			Msg("failed to Close request body")
	}

	kv = make([]*roxypb.KeyValue, 0, len(rc.Request.Trailer))
	for trailerName, trailerValues := range rc.Request.Trailer {
		lc := strings.ToLower(trailerName)
		for _, trailerValue := range trailerValues {
			kv = append(kv, &roxypb.KeyValue{Key: lc, Value: trailerValue})
		}
	}

	if len(kv) != 0 {
		err = sc.Send(&roxypb.WebMessage{Trailers: kv})
		if err != nil {
			rc.Logger.Error().
				Err(err).
				Msg("/roxy.WebServer/Serve: Send failed")
			return
		}
	}

	needCloseSend = false
	err = sc.CloseSend()
	if err != nil {
		rc.Logger.Warn().
			Err(err).
			Msg("/roxy.WebServer/Server: CloseSend failed")
		return
	}
}

func (h *GRPCBackendHandler) recvThread(rc *RequestContext, sc roxypb.WebServer_ServeClient, wg *sync.WaitGroup) {
	needWriteHeader := true
	defer func() {
		if needWriteHeader {
			rc.Writer.WriteHeader(http.StatusOK)
		}
		wg.Done()
	}()

Loop:
	for {
		resp, err := sc.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			needWriteHeader = false
			rc.Writer.WriteError(http.StatusInternalServerError)
			rc.Logger.Error().
				Err(err).
				Msg("/roxy.WebServer/Serve: Recv failed")
			return
		}

		if len(resp.Headers) != 0 && needWriteHeader {
			hdrs := rc.Writer.Header()
			status := ""
			for _, kv := range resp.Headers {
				switch kv.Key {
				case ":status":
					status = kv.Value
				default:
					hdrs.Add(kv.Key, kv.Value)
				}
			}
			statusCode := http.StatusOK
			if status != "" {
				statusCode, err = strconv.Atoi(status)
				if err != nil {
					statusCode = http.StatusInternalServerError
					rc.Logger.Warn().
						Str("arg", status).
						Err(err).
						Msg("/roxy.WebServer/Serve: failed to parse \":status\" pseudo-header")
				}
			}
			needWriteHeader = false
			rc.Writer.WriteHeader(statusCode)
		}

		if len(resp.BodyChunk) != 0 {
			needWriteHeader = false
			_, err = rc.Writer.Write(resp.BodyChunk)
			if err != nil {
				rc.Logger.Error().
					Err(err).
					Msg("failed to Write response body")
				break Loop
			}
		}

		if len(resp.Trailers) != 0 {
			if needWriteHeader {
				needWriteHeader = false
				rc.Writer.WriteHeader(http.StatusOK)
			}
			hdrs := rc.Writer.Header()
			for _, kv := range resp.Trailers {
				hdrs.Add(kv.Key, kv.Value)
			}
		}
	}

	// This loop is only executed if ResponseWriter.Write failed.
	for {
		_, err := sc.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			rc.Logger.Error().
				Err(err).
				Msg("/roxy.WebServer/Serve: Recv failed")
			return
		}
	}
}

func (h *GRPCBackendHandler) Close() error {
	return h.cc.Close()
}

var _ http.Handler = (*GRPCBackendHandler)(nil)

// }}}

func CompileErrorHandler(impl *Impl, arg string) (http.Handler, error) {
	// format: "ERROR:<status>"

	str := arg[6:]

	u64, err := strconv.ParseUint(str, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uint %q: %w", str, err)
	}
	if u64 < 400 || u64 >= 600 {
		return nil, fmt.Errorf("status %d out of range [400..599]", u64)
	}
	statusCode := int(u64)

	return ErrorHandler{Status: statusCode}, nil
}

func CompileRedirHandler(impl *Impl, arg string) (http.Handler, error) {
	// format: "REDIR:<status>:https://example.com/path?query"

	statusStr := arg[6:9]
	templateStr := arg[10:]

	u64, err := strconv.ParseUint(statusStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uint %q: %w", statusStr, err)
	}
	if u64 < 300 || u64 >= 400 {
		return nil, fmt.Errorf("status %d out of range [300..399]", u64)
	}
	statusCode := int(u64)

	t := template.New("redir")
	t, err = t.Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to compile text/template %q: %w", templateStr, err)
	}
	return RedirHandler{Template: t, Status: statusCode}, nil
}

func CompileTarget(impl *Impl, key string, cfg *TargetConfig) (http.Handler, error) {
	switch cfg.Type {
	case enums.UndefinedTargetType:
		return nil, fmt.Errorf("missing required field \"type\"")

	case enums.FileSystemTargetType:
		return CompileFileSystemHandler(impl, key, cfg)

	case enums.HTTPBackendTargetType:
		return CompileHTTPBackendHandler(impl, key, cfg)

	case enums.GRPCBackendTargetType:
		return CompileGRPCBackendHandler(impl, key, cfg)

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

	return &FileSystemHandler{fs}, nil
}

func CompileHTTPBackendHandler(impl *Impl, key string, cfg *TargetConfig) (http.Handler, error) {
	tlsConfig, err := CompileTLSClientConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}

	parsedTarget, err := baseresolver.ParseTargetString(cfg.Target)
	if err != nil {
		return nil, err
	}

	bc, err := balancedclient.New(balancedclient.Options{
		Context:   gRootContext,
		Target:    parsedTarget,
		Etcd:      impl.etcd,
		ZK:        impl.zkconn,
		Dialer:    &gDialer,
		TLSConfig: tlsConfig,
	})
	if err != nil {
		return nil, err
	}
	return &HTTPBackendHandler{bc}, nil
}

func CompileGRPCBackendHandler(impl *Impl, key string, cfg *TargetConfig) (http.Handler, error) {
	tlsConfig, err := CompileTLSClientConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}

	resolvers := make([]resolver.Builder, 0, 3)
	resolvers = append(resolvers, atcresolver.NewBuilder(gRootContext, nil))
	if impl.etcd != nil {
		resolvers = append(resolvers, etcdresolver.NewBuilder(gRootContext, nil, impl.etcd, ""))
	}
	if impl.zkconn != nil {
		resolvers = append(resolvers, zkresolver.NewBuilder(gRootContext, nil, impl.zkconn, ""))
	}

	dialOpts := make([]grpc.DialOption, 1, 2)
	if tlsConfig != nil {
		dialOpts[0] = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	} else {
		dialOpts[0] = grpc.WithInsecure()
	}
	if len(resolvers) != 0 {
		dialOpts = append(dialOpts, grpc.WithResolvers(resolvers...))
	}

	cc, err := grpc.DialContext(gRootContext, cfg.Target, dialOpts...)
	if err != nil {
		return nil, err
	}

	return &GRPCBackendHandler{cc}, nil
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
		if isIPv6(tcpAddr.IP) {
			return strconv.Quote(tcpAddr.String())
		}
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

// type headerNames {{{

type headerNames []string

func (list headerNames) Len() int {
	return len(list)
}

func (list headerNames) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list headerNames) Less(i, j int) bool {
	a := strings.ToLower(list[i])
	b := strings.ToLower(list[j])
	return a < b
}

func (list headerNames) Sort() {
	sort.Sort(list)
}

// }}}
