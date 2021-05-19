package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
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
	"path"
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
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/internal/balancedclient"
	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/enums"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

const (
	problemNone = "none"
	problem4xx  = "4xx"
	problem5xx  = "5xx"
	problemErr  = "error"
)

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

var (
	gFileCacheMu    sync.Mutex
	gFileCacheMap   map[cacheKey]cacheRow = make(map[cacheKey]cacheRow, 1024)
	gFileCacheBytes int64
)

var (
	promFileCacheCount = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "roxy",
			Subsystem: constants.SubsystemHTTPS,
			Name:      "cache_files",
			Help:      "number of files in the in-RAM cache",
		},
		func() float64 {
			gFileCacheMu.Lock()
			x := len(gFileCacheMap)
			gFileCacheMu.Unlock()
			return float64(x)
		},
	)

	promFileCacheBytes = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "roxy",
			Subsystem: constants.SubsystemHTTPS,
			Name:      "cache_bytes",
			Help:      "number of bytes in the in-RAM cache",
		},
		func() float64 {
			gFileCacheMu.Lock()
			x := gFileCacheBytes
			gFileCacheMu.Unlock()
			return float64(x)
		},
	)
)

var gMetrics = map[string]*Metrics{
	constants.SubsystemProm:  NewMetrics(constants.SubsystemProm),
	constants.SubsystemHTTP:  NewMetrics(constants.SubsystemHTTP),
	constants.SubsystemHTTPS: NewMetrics(constants.SubsystemHTTPS),
}

func init() {
	for _, metrics := range gMetrics {
		metrics.MustRegister(prometheus.DefaultRegisterer)
	}
	prometheus.DefaultRegisterer.MustRegister(promFileCacheCount)
	prometheus.DefaultRegisterer.MustRegister(promFileCacheBytes)
}

// type Metrics {{{

type Metrics struct {
	PanicCount                     prometheus.Counter
	RequestCountByMethod           *prometheus.CounterVec
	ResponseCountByCode            *prometheus.CounterVec
	RequestSize                    prometheus.Histogram
	ResponseSize                   prometheus.Histogram
	ResponseDuration               prometheus.Histogram
	ResponseCountByCodeAndFrontend *prometheus.CounterVec
	ProblemCountByTypeAndFrontend  *prometheus.CounterVec
	ResponseSizeByFrontend         *prometheus.HistogramVec
	ResponseDurationByFrontend     *prometheus.HistogramVec
}

func NewMetrics(subsystem string) *Metrics {
	m := new(Metrics)

	m.PanicCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "roxy",
			Subsystem: subsystem,
			Name:      "panics_total",
			Help:      "the number of HTTP requests whose handlers paniced",
		},
	)

	m.RequestCountByMethod = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "roxy",
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "the number of incoming HTTP requests",
		},
		[]string{"method"},
	)

	m.ResponseCountByCode = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "roxy",
			Subsystem: subsystem,
			Name:      "responses_total",
			Help:      "the number of outgoing HTTP responses",
		},
		[]string{"code"},
	)

	m.RequestSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "roxy",
			Subsystem: subsystem,
			Name:      "request_size_bytes",
			Help:      "the size of the incoming HTTP request",
			Buckets:   prometheus.ExponentialBuckets(1024, 4, 10),
		},
	)

	m.ResponseSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "roxy",
			Subsystem: subsystem,
			Name:      "response_size_bytes",
			Help:      "the size of the outgoing HTTP response",
			Buckets:   prometheus.ExponentialBuckets(1024, 4, 10),
		},
	)

	m.ResponseDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "roxy",
			Subsystem: subsystem,
			Name:      "response_duration_seconds",
			Help:      "the duration of the HTTP request lifetime",
			Buckets:   prometheus.ExponentialBuckets(0.001, 10, 7),
		},
	)

	if subsystem == constants.SubsystemHTTPS {
		m.ResponseCountByCodeAndFrontend = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "roxy",
				Subsystem: subsystem,
				Name:      "frontend_responses_total",
				Help:      "the number of outgoing HTTP responses",
			},
			[]string{"code", "frontend"},
		)

		m.ProblemCountByTypeAndFrontend = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "roxy",
				Subsystem: subsystem,
				Name:      "frontend_problems_total",
				Help:      "the number of failed HTTP responses",
			},
			[]string{"problem", "frontend"},
		)

		m.ResponseSizeByFrontend = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "roxy",
				Subsystem: subsystem,
				Name:      "frontend_response_size_bytes",
				Help:      "the size of the outgoing HTTP response",
				Buckets:   prometheus.ExponentialBuckets(1024, 4, 10),
			},
			[]string{"frontend"},
		)

		m.ResponseDurationByFrontend = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "roxy",
				Subsystem: subsystem,
				Name:      "frontend_response_duration_seconds",
				Help:      "the duration of the HTTP request lifetime",
				Buckets:   prometheus.ExponentialBuckets(0.001, 10, 7),
			},
			[]string{"frontend"},
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
	if m.ResponseCountByCodeAndFrontend != nil {
		out = append(
			out,
			m.ResponseCountByCodeAndFrontend,
			m.ProblemCountByTypeAndFrontend,
			m.ResponseSizeByFrontend,
			m.ResponseDurationByFrontend,
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
	r.Header.Set(constants.HeaderXID, idStr)
	w.Header().Set(constants.HeaderXID, idStr)
	w.Header().Set(constants.HeaderServer, "roxy/"+mainutil.RoxyVersion())
	w.Header().Set(constants.HeaderCSP, "default-src 'self';")
	w.Header().Set(constants.HeaderHSTS, "max-age=86400")
	w.Header().Set(constants.HeaderXCTO, "nosniff")
	w.Header().Set(constants.HeaderXXSSP, "1; mode=block")

	ctx := r.Context()
	cc := mainutil.GetConnContext(ctx)
	impl := h.Ref.Get()

	c := cc.Logger.With()
	c = c.Str("xid", idStr)
	c = c.Str("ip", addrWithNoPort(cc.RemoteAddr))
	c = c.Str("method", r.Method)
	c = c.Str("host", r.Host)
	c = c.Str("url", r.URL.String())
	if value := r.Header.Get(constants.HeaderUserAgent); value != "" {
		c = c.Str("userAgent", value)
	}
	if value := r.Header.Get(constants.HeaderReferer); value != "" {
		c = c.Str("referer", value)
	}

	rc := &RequestContext{
		Logger:     c.Logger(),
		Subsystem:  cc.Subsystem,
		LocalAddr:  cc.LocalAddr,
		RemoteAddr: cc.RemoteAddr,
		XID:        id,
		Impl:       impl,
		Metrics:    gMetrics[cc.Subsystem],
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
			_, _ = io.Copy(io.Discard, rc.Body)
			_ = rc.Body.Close()
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
			problemType = problemErr
		case rc.Writer.Status() >= 500:
			problemType = problem5xx
		case rc.Writer.Status() >= 400:
			problemType = problem4xx
		default:
			problemType = problemNone
		}

		rc.Metrics.ResponseCountByCode.WithLabelValues(code).Inc()
		rc.Metrics.RequestSize.Observe(bytesRead)
		rc.Metrics.ResponseSize.Observe(bytesWritten)
		rc.Metrics.ResponseDuration.Observe(duration)

		if rc.Metrics.ResponseCountByCodeAndFrontend != nil {
			frontendKey := simplifyFrontendKey(rc.FrontendKey)
			rc.Metrics.ResponseCountByCodeAndFrontend.WithLabelValues(code, frontendKey).Inc()
			rc.Metrics.ProblemCountByTypeAndFrontend.WithLabelValues(problemType, frontendKey).Inc()
			rc.Metrics.ResponseSizeByFrontend.WithLabelValues(frontendKey).Observe(bytesWritten)
			rc.Metrics.ResponseDurationByFrontend.WithLabelValues(frontendKey).Observe(duration)
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
		} else if rc.Subsystem == constants.SubsystemProm {
			event = rc.Logger.Debug()
		} else {
			event = rc.Logger.Info()
		}
		event = event.Dur("elapsed", elapsed)
		event = event.Int("status", rc.Writer.Status())
		if location := rc.Writer.Header().Get(constants.HeaderLocation); location != "" {
			event = event.Str("location", location)
		}
		if contentType := rc.Writer.Header().Get(constants.HeaderContentType); contentType != "" {
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
	rc.FrontendKey = "!HTTP"

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
			rc.FrontendKey = lastRule.FrontendKey
			rc.FrontendConfig = lastRule.FrontendConfig
			rc.Logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
				c = c.Str("key", simplifyFrontendKey(lastRule.FrontendKey))
				if lastRule.FrontendConfig != nil {
					c = c.Interface("frontend", lastRule.FrontendConfig)
				}
				return c
			})
			lastRule.FrontendHandler.ServeHTTP(w, r)
			return
		}
	}

	rc.FrontendKey = "!EOF"
	rc.Writer.WriteError(http.StatusNotFound)
	rc.Request.URL.Scheme = constants.SchemeHTTPS
	rc.Request.URL.Host = rc.Request.Host
	rc.Logger.Warn().
		Str("mutatedURL", rc.Request.URL.String()).
		Msg("no matching frontend")
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
	u.Scheme = constants.SchemeHTTPS
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
	hdrs.Set(constants.HeaderCacheControl, constants.CacheControlPublic1Day)

	if rc.Request.Method == http.MethodOptions {
		hdrs.Set(constants.HeaderAllow, constants.AllowGET)
		rc.Writer.WriteHeader(http.StatusNoContent)
		return
	}

	if rc.Request.Method != http.MethodHead && rc.Request.Method != http.MethodGet {
		hdrs.Set(constants.HeaderAllow, constants.AllowGET)
		hdrs.Del(constants.HeaderCacheControl)
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

	defer func() {
		_ = reqFile.Close()
	}()

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
					_ = idxFile.Close()
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
	hdrs.Set(constants.HeaderContentType, contentType)
	if contentLang != "" {
		hdrs.Set(constants.HeaderContentLang, contentLang)
	}
	if contentEnc != "" {
		hdrs.Set(constants.HeaderContentEnc, contentEnc)
	}

	if contentType == constants.ContentTypeAppJS || contentType == constants.ContentTypeTextJS || strings.HasPrefix(contentType, constants.ContentTypeTextJS+";") {
		mapFilePath := rc.Request.URL.Path + ".map"
		mapFile, err := h.fs.Open(mapFilePath)
		if err == nil {
			_ = mapFile.Close()
			hdrs.Set(constants.HeaderSourceMap, mapFilePath)
		}
	}

	var maxCacheSize int64 = defaultMaxCacheSize
	if rc.Impl.cfg.Global != nil && rc.Impl.cfg.Global.MaxCacheSize != 0 {
		maxCacheSize = rc.Impl.cfg.Global.MaxCacheSize
	}

	var maxComputeDigestSize int64 = defaultMaxComputeDigestSize
	if rc.Impl.cfg.Global != nil && rc.Impl.cfg.Global.MaxComputeDigestSize != 0 {
		maxComputeDigestSize = rc.Impl.cfg.Global.MaxComputeDigestSize
	}

	size := fi.Size()
	mtime := fi.ModTime()
	if size <= maxCacheSize {
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
				if existing, found := gFileCacheMap[key]; found {
					gFileCacheBytes -= int64(len(existing.bytes))
				}
				gFileCacheMap[key] = row
				gFileCacheBytes += int64(len(row.bytes))
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
			hdrs.Set(constants.HeaderETag, etag)
		}

		if !haveMD5 && !haveSHA1 && !haveSHA256 && fi.Size() <= maxComputeDigestSize {
			if _, err := f.Seek(0, io.SeekStart); err == nil {
				md5writer := md5.New()
				sha1writer := sha1.New()
				sha256writer := sha256.New()

				mw := io.MultiWriter(md5writer, sha1writer, sha256writer)
				_, err = io.Copy(mw, f)
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
	hdrs.Set(constants.HeaderContentLen, strconv.FormatInt(size, 10))

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

	var cache misc.LookupCache

	entries := make([]DirEntry, 0, 1+len(list))
	if rc.Request.URL.Path != "/" {
		var e DirEntry

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
				populateRealStats(rc.Impl, rc.Logger, h.fs, f, &cache, &e, parentInfo, parentDir)
			}
			_ = parentFile.Close()
		}

		entries = append(entries, e)
	}
	for _, fi := range list {
		var e DirEntry

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

		populateRealStats(rc.Impl, rc.Logger, h.fs, f, &cache, &e, fi, path.Join(rc.Request.URL.Path, e.Name))

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
		Entries          []DirEntry
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

	hdrs.Set(constants.HeaderContentType, page.contentType)
	if page.contentLang != "" {
		hdrs.Set(constants.HeaderContentLang, page.contentLang)
	}
	if page.contentEnc != "" {
		hdrs.Set(constants.HeaderContentEnc, page.contentEnc)
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

	rc.Request.Header.Set(constants.HeaderRoxyFrontend, rc.RoxyFrontend())
	rc.Request.Header.Set(constants.HeaderXFwdHost, rc.Request.Host)
	rc.Request.Header.Set(constants.HeaderXFwdProto, constants.SchemeHTTPS)
	rc.Request.Header.Set(constants.HeaderXFwdIP, addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add(constants.HeaderXFwdFor, addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add(
		constants.HeaderForwarded,
		fmt.Sprintf("by=%s;for=%s;host=%s;proto=%s",
			quoteForwardedAddr(rc.LocalAddr),
			quoteForwardedAddr(rc.RemoteAddr),
			rc.Request.Host,
			constants.SchemeHTTPS))

	if values := rc.Request.Header.Values(constants.HeaderXFwdFor); len(values) > 1 {
		rc.Request.Header.Set(constants.HeaderXFwdFor, strings.Join(values, ", "))
	}

	innerURL := new(url.URL)
	*innerURL = *rc.Request.URL
	innerURL.Scheme = constants.SchemeHTTP
	innerURL.Host = "example.com"
	if h.client.IsTLS() {
		innerURL.Scheme = constants.SchemeHTTPS
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
			_ = innerResp.Body.Close()
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
	cc  *grpc.ClientConn
	web roxy_v0.WebClient
}

func (h *GRPCBackendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := GetRequestContext(ctx)

	rc.Request.Header.Set(constants.HeaderRoxyFrontend, rc.RoxyFrontend())
	rc.Request.Header.Set(constants.HeaderXFwdHost, rc.Request.Host)
	rc.Request.Header.Set(constants.HeaderXFwdProto, constants.SchemeHTTPS)
	rc.Request.Header.Set(constants.HeaderXFwdIP, addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add(constants.HeaderXFwdFor, addrWithNoPort(rc.RemoteAddr))
	rc.Request.Header.Add(
		constants.HeaderForwarded,
		fmt.Sprintf("by=%s;for=%s;host=%s;proto=%s",
			quoteForwardedAddr(rc.LocalAddr),
			quoteForwardedAddr(rc.RemoteAddr),
			rc.Request.Host,
			constants.SchemeHTTPS))

	var wg sync.WaitGroup
	defer func() {
		if rc.Body.WasClosed() {
			_, _ = io.Copy(io.Discard, rc.Body)
			_ = rc.Body.Close()
		}
		wg.Wait()
	}()

	sc, err := h.web.Serve(ctx)
	if err != nil {
		rc.Writer.WriteError(http.StatusServiceUnavailable)
		rc.Logger.Error().
			Str("rpcService", "roxy.v0.Web").
			Str("rpcMethod", "Serve").
			Err(err).
			Msg("RPC failed")
		return
	}

	needCloseSend := true
	defer func() {
		if needCloseSend {
			_ = sc.CloseSend()
		}
	}()

	wg.Add(1)
	go h.recvThread(rc, sc, &wg)

	kv := make([]*roxy_v0.KeyValue, 4, 4+len(rc.Request.Header))
	kv[0] = &roxy_v0.KeyValue{Key: constants.PseudoHeaderScheme, Value: constants.SchemeHTTPS}
	kv[1] = &roxy_v0.KeyValue{Key: constants.PseudoHeaderMethod, Value: rc.Request.Method}
	kv[2] = &roxy_v0.KeyValue{Key: constants.PseudoHeaderAuthority, Value: rc.Request.Host}
	kv[3] = &roxy_v0.KeyValue{Key: constants.PseudoHeaderPath, Value: rc.Request.URL.Path}
	kv = appendHeadersToKV(kv, rc.Request.Header)
	err = sc.Send(&roxy_v0.WebMessage{Headers: kv})
	if err != nil {
		rc.Logger.Error().
			Str("rpcService", "roxy.v0.Web").
			Str("rpcMethod", "Serve").
			Err(err).
			Msg("RPC Send failed")
		return
	}

	buf := make([]byte, 1<<20) // 1 MiB
	for {
		n, err := io.ReadFull(rc.Body, buf)
		if n > 0 {
			err2 := sc.Send(&roxy_v0.WebMessage{BodyChunk: buf[:n]})
			if err2 != nil {
				rc.Logger.Error().
					Str("rpcService", "roxy.v0.Web").
					Str("rpcMethod", "Serve").
					Err(err).
					Msg("RPC Send failed")
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

	err = rc.Body.Close()
	if err != nil {
		rc.Logger.Warn().
			Err(err).
			Msg("failed to Close request body")
	}

	kv = make([]*roxy_v0.KeyValue, 0, len(rc.Request.Trailer))
	kv = appendHeadersToKV(kv, rc.Request.Trailer)
	if len(kv) != 0 {
		err = sc.Send(&roxy_v0.WebMessage{Trailers: kv})
		if err != nil {
			rc.Logger.Error().
				Str("rpcService", "roxy.v0.Web").
				Str("rpcMethod", "Serve").
				Err(err).
				Msg("RPC Send failed")
			return
		}
	}

	needCloseSend = false
	err = sc.CloseSend()
	if err != nil {
		rc.Logger.Warn().
			Str("rpcService", "roxy.v0.Web").
			Str("rpcMethod", "Serve").
			Err(err).
			Msg("RPC CloseSend failed")
		return
	}
}

func (h *GRPCBackendHandler) recvThread(rc *RequestContext, sc roxy_v0.Web_ServeClient, wg *sync.WaitGroup) {
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
				Str("rpcService", "roxy.v0.Web").
				Str("rpcMethod", "Serve").
				Err(err).
				Msg("RPC Recv failed")
			return
		}

		if len(resp.Headers) != 0 && needWriteHeader {
			hdrs := rc.Writer.Header()
			status := ""
			for _, kv := range resp.Headers {
				switch kv.Key {
				case constants.PseudoHeaderStatus:
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
						Str("rpcService", "roxy.v0.Web").
						Str("rpcMethod", "Serve").
						Str("input", status).
						Err(err).
						Msg("RPC Recv: failed to parse \":status\" pseudo-header")
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
				Str("rpcService", "roxy.v0.Web").
				Str("rpcMethod", "Serve").
				Err(err).
				Msg("RPC Recv failed")
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

func CompileFrontend(impl *Impl, key string, cfg *FrontendConfig) (http.Handler, error) {
	switch cfg.Type {
	case enums.UndefinedFrontendType:
		return nil, fmt.Errorf("missing required field \"type\"")

	case enums.FileSystemFrontendType:
		return CompileFileSystemHandler(impl, key, cfg)

	case enums.HTTPBackendFrontendType:
		return CompileHTTPBackendHandler(impl, key, cfg)

	case enums.GRPCBackendFrontendType:
		return CompileGRPCBackendHandler(impl, key, cfg)

	default:
		panic(fmt.Errorf("unknown frontend type %#v", cfg.Type))
	}
}

func CompileFileSystemHandler(impl *Impl, key string, cfg *FrontendConfig) (http.Handler, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("missing required field \"path\"")
	}

	abs, err := roxyutil.ExpandPath(cfg.Path)
	if err != nil {
		return nil, err
	}

	var fs http.FileSystem = http.Dir(abs)

	return &FileSystemHandler{fs}, nil
}

func CompileHTTPBackendHandler(impl *Impl, key string, cfg *FrontendConfig) (http.Handler, error) {
	gcc := cfg.Client

	tlsConfig, err := gcc.TLS.MakeTLS(key)
	if err != nil {
		return nil, err
	}

	res, err := roxyresolver.New(roxyresolver.Options{
		Context: impl.ctx,
		Target:  gcc.Target,
		IsTLS:   (tlsConfig != nil),
	})
	if err != nil {
		return nil, err
	}

	interceptor := atcclient.InterceptorFactory{
		DefaultCostPerQuery:   1,
		DefaultCostPerRequest: 1,
	}

	bc, err := balancedclient.New(res, interceptor, &gDialer, tlsConfig)
	if err != nil {
		return nil, err
	}
	return &HTTPBackendHandler{bc}, nil
}

func CompileGRPCBackendHandler(impl *Impl, key string, cfg *FrontendConfig) (http.Handler, error) {
	gcc := cfg.Client

	interceptor := atcclient.InterceptorFactory{
		DefaultCostPerQuery:   1,
		DefaultCostPerRequest: 1,
	}

	cc, err := gcc.Dial(impl.ctx, interceptor.DialOptions()...)
	if err != nil {
		return nil, err
	}

	web := roxy_v0.NewWebClient(cc)

	return &GRPCBackendHandler{cc, web}, nil
}

type DirEntry struct {
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

func populateRealStats(impl *Impl, logger zerolog.Logger, rootFS http.FileSystem, f http.File, cache *misc.LookupCache, e *DirEntry, fi fs.FileInfo, fullPath string) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		e.Dev = st.Dev
		e.Ino = st.Ino
		e.NLink = uint32(st.Nlink)
		e.Owner = userNameByID(cache, st.Uid)
		e.Group = groupNameByID(cache, st.Gid)
		e.Mode = modeString(st.Mode)

		if (st.Mode & syscall.S_IFMT) == syscall.S_IFLNK {
			e.IsLink = true
			if link, err := readLinkAt(f, fi.Name()); err == nil {
				e.Link = link
			}
		}
	}

	var (
		contentType string
		contentLang string
		contentEnc  string
	)
	if e.IsDir {
		contentType = "inode/directory"
	} else if !e.IsLink {
		contentType, contentLang, contentEnc = DetectMimeProperties(impl, logger, rootFS, fullPath)
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

func userNameByID(cache *misc.LookupCache, uid uint32) string {
	u, err := cache.UserByID(uid)
	if err != nil {
		return fmt.Sprintf("#%d", uid)
	}
	return u.Username
}

func groupNameByID(cache *misc.LookupCache, gid uint32) string {
	g, err := cache.GroupByID(gid)
	if err != nil {
		return fmt.Sprintf("#%d", gid)
	}
	return g.Name
}

func modeString(unixMode uint32) string {
	var realMode [10]byte
	for i := 0; i < 10; i++ {
		realMode[i] = '-'
	}

	switch unixMode & syscall.S_IFMT {
	case syscall.S_IFREG:
		realMode[0] = '-'
	case syscall.S_IFDIR:
		realMode[0] = 'd'
	case syscall.S_IFBLK:
		realMode[0] = 'b'
	case syscall.S_IFCHR:
		realMode[0] = 'c'
	case syscall.S_IFIFO:
		realMode[0] = 'p'
	case syscall.S_IFSOCK:
		realMode[0] = 's'
	case syscall.S_IFLNK:
		realMode[0] = 'l'
	default:
		realMode[0] = '?'
	}

	if (unixMode & 0400) == 0400 {
		realMode[1] = 'r'
	}
	if (unixMode & 0200) == 0200 {
		realMode[2] = 'w'
	}
	switch unixMode & 04100 {
	case 04100:
		realMode[3] = 's'
	case 04000:
		realMode[3] = 'S'
	case 00100:
		realMode[3] = 'x'
	}

	if (unixMode & 0040) == 0040 {
		realMode[4] = 'r'
	}
	if (unixMode & 0020) == 0020 {
		realMode[5] = 'w'
	}
	switch unixMode & 02010 {
	case 02010:
		realMode[6] = 's'
	case 02000:
		realMode[6] = 'S'
	case 00010:
		realMode[6] = 'x'
	}

	if (unixMode & 0004) == 0004 {
		realMode[7] = 'r'
	}
	if (unixMode & 0002) == 0002 {
		realMode[8] = 'w'
	}
	switch unixMode & 01001 {
	case 01001:
		realMode[9] = 't'
	case 01000:
		realMode[9] = 'T'
	case 00001:
		realMode[9] = 'x'
	}

	return string(realMode[:])
}
