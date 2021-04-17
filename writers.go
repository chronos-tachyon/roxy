package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	htmltemplate "html/template"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// type WrappedWriter {{{

type WrappedWriter interface {
	http.ResponseWriter
	http.Pusher
	Unwrap() http.ResponseWriter
	Status() int
	BytesWritten() int64
	SetIsHEAD(value bool)
	SetRules(rules []*Rule, req *http.Request)
	Redirect(ctx context.Context, statusCode int, urlstr string)
	Error(ctx context.Context, statusCode int)
	SawError() bool
}

// type BasicWriter {{{

type BasicWriter struct {
	next        http.ResponseWriter
	wroteHeader bool
	isHEAD      bool
	sawError    bool
	status      int
	bytes       int64
	rules       []*Rule
	request     *http.Request
}

func (bw *BasicWriter) noResponseBody() bool {
	return bw.isHEAD || bw.status == http.StatusNoContent
}

func (bw *BasicWriter) Header() http.Header {
	return bw.next.Header()
}

func (bw *BasicWriter) WriteHeader(status int) {
	if !bw.wroteHeader {
		bw.status = status
		bw.wroteHeader = true

		for _, rule := range bw.rules {
			rule.ApplyPost(bw, bw.request)
		}

		bw.next.WriteHeader(status)
	}
}

func (bw *BasicWriter) Write(buf []byte) (int, error) {
	bw.WriteHeader(http.StatusOK)
	if bw.noResponseBody() {
		return len(buf), nil
	}
	n, err := bw.next.Write(buf)
	bw.bytes += int64(n)
	return n, err
}

func (bw *BasicWriter) Push(target string, opts *http.PushOptions) error {
	if x, ok := bw.next.(http.Pusher); ok {
		return x.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (bw *BasicWriter) Unwrap() http.ResponseWriter {
	return bw.next
}

func (bw *BasicWriter) Status() int {
	return bw.status
}

func (bw *BasicWriter) BytesWritten() int64 {
	return bw.bytes
}

func (bw *BasicWriter) SetIsHEAD(value bool) {
	bw.isHEAD = value
}

func (bw *BasicWriter) SetRules(rules []*Rule, req *http.Request) {
	bw.rules = rules
	bw.request = req
}

func (bw *BasicWriter) Redirect(ctx context.Context, statusCode int, urlstr string) {
	if statusCode < 300 || statusCode >= 400 {
		panic(fmt.Errorf("HTTP status code %03d out of range [300..399]", statusCode))
	}

	if u, err := url.Parse(urlstr); err == nil {
		if u.Scheme == "" && u.Host == "" {
			oldPath := bw.request.URL.Path
			if oldPath == "" {
				oldPath = "/"
			}

			if urlstr == "" || urlstr[0] != '/' {
				oldDir, _ := path.Split(oldPath)
				urlstr = oldDir + urlstr
			}

			var query string
			if index := strings.IndexByte(urlstr, '?'); index >= 0 {
				urlstr, query = urlstr[:index], urlstr[index:]
			}

			hadTrailingSlash := strings.HasSuffix(urlstr, "/")
			urlstr = path.Clean(urlstr)
			if hadTrailingSlash {
				urlstr += "/"
			}

			urlstr += query
		}
	}

	u, err := url.Parse(urlstr)
	if err != nil {
		panic(err)
	}

	hdrs := bw.Header()
	purgeContentHeaders(hdrs)
	hdrs.Set("location", hexEscapeNonASCII(urlstr))
	hdrs.Set("cache-control", "max-age=86400")

	haveImpl := true
	if value := ctx.Value(implKey{}); value == nil {
		haveImpl = false
	}

	if bw.request.Method != http.MethodHead && bw.request.Method != http.MethodGet {
		return
	}

	if haveImpl {
		impl := implFromCtx(ctx)
		if impl.cfg.ErrorPages != nil && impl.cfg.ErrorPages.Root != "" {
			fullPath := getErrorPagePath(impl, statusCode)
			if ok := tryWriteErrorPage(bw, impl, statusCode, u, fullPath); ok {
				return
			}

			partialPath, found := impl.cfg.ErrorPages.Map["redir"]
			if !found {
				partialPath = "redir.tmpl"
			}
			fullPath = filepath.Join(impl.cfg.ErrorPages.Root, partialPath)
			if ok := tryWriteErrorPage(bw, impl, statusCode, u, fullPath); ok {
				return
			}
		}
	}

	tryWriteErrorPage2(bw, nil, statusCode, u, defaultRedirPageTemplate)
}

func (bw *BasicWriter) Error(ctx context.Context, statusCode int) {
	if statusCode < 400 || statusCode >= 600 {
		panic(fmt.Errorf("HTTP status code %03d out of range [400..599]", statusCode))
	}

	bw.sawError = true
	if bw.wroteHeader {
		return
	}

	hdrs := bw.Header()
	purgeContentHeaders(hdrs)
	hdrs.Set("cache-control", "no-cache")

	haveImpl := true
	if value := ctx.Value(implKey{}); value == nil {
		haveImpl = false
	}

	if haveImpl {
		impl := implFromCtx(ctx)
		if impl.cfg.ErrorPages != nil && impl.cfg.ErrorPages.Root != "" {
			fullPath := getErrorPagePath(impl, statusCode)
			if ok := tryWriteErrorPage(bw, impl, statusCode, nil, fullPath); ok {
				return
			}

			partialPath, found := impl.cfg.ErrorPages.Map["error"]
			if !found {
				partialPath = "error.tmpl"
			}
			fullPath = filepath.Join(impl.cfg.ErrorPages.Root, partialPath)
			if ok := tryWriteErrorPage(bw, impl, statusCode, nil, fullPath); ok {
				return
			}
		}
	}

	tryWriteErrorPage2(bw, nil, statusCode, nil, defaultErrorPageTemplate)
}

func (bw *BasicWriter) SawError() bool {
	return bw.sawError
}

var (
	_ http.ResponseWriter = (*BasicWriter)(nil)
	_ http.Pusher         = (*BasicWriter)(nil)
	_ WrappedWriter       = (*BasicWriter)(nil)
)

// }}}

// type FlushWriter {{{

type FlushWriter struct {
	BasicWriter
}

func (fw *FlushWriter) Flush() {
	fw.BasicWriter.next.(http.Flusher).Flush()
}

var (
	_ http.ResponseWriter = (*FlushWriter)(nil)
	_ http.Pusher         = (*FlushWriter)(nil)
	_ http.Flusher        = (*FlushWriter)(nil)
	_ WrappedWriter       = (*FlushWriter)(nil)
)

// }}}

// type FancyWriter {{{

type FancyWriter struct {
	BasicWriter
}

func (fw *FancyWriter) Flush() {
	fw.BasicWriter.next.(http.Flusher).Flush()
}

func (fw *FancyWriter) CloseNotify() <-chan bool {
	return fw.BasicWriter.next.(http.CloseNotifier).CloseNotify()
}

func (fw *FancyWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return fw.BasicWriter.next.(http.Hijacker).Hijack()
}

func (fw *FancyWriter) ReadFrom(r io.Reader) (int64, error) {
	fw.BasicWriter.WriteHeader(http.StatusOK)
	return fw.BasicWriter.next.(io.ReaderFrom).ReadFrom(r)
}

var (
	_ http.ResponseWriter = (*FancyWriter)(nil)
	_ http.Pusher         = (*FancyWriter)(nil)
	_ http.Flusher        = (*FancyWriter)(nil)
	_ http.CloseNotifier  = (*FancyWriter)(nil)
	_ http.Hijacker       = (*FancyWriter)(nil)
	_ io.ReaderFrom       = (*FancyWriter)(nil)
	_ WrappedWriter       = (*FancyWriter)(nil)
)

// }}}

// }}}

func WrapWriter(w http.ResponseWriter) WrappedWriter {
	type fancyInterface interface {
		http.ResponseWriter
		http.Flusher
		http.CloseNotifier
		http.Hijacker
		io.ReaderFrom
	}

	if _, ok := w.(fancyInterface); ok {
		return &FancyWriter{BasicWriter{next: w}}
	}

	type flushInterface interface {
		http.ResponseWriter
		http.Flusher
	}

	if _, ok := w.(flushInterface); ok {
		return &FlushWriter{BasicWriter{next: w}}
	}

	return &BasicWriter{next: w}
}

func hexEscapeNonASCII(str string) string {
	var newLen uint
	for i, j := uint(0), uint(len(str)); i < j; i++ {
		ch := str[i]
		if ch >= 0x80 {
			newLen += 3
		} else {
			newLen++
		}
	}
	if newLen == uint(len(str)) {
		return str
	}
	buf := make([]byte, 0, newLen)
	for i, j := uint(0), uint(len(str)); i < j; i++ {
		ch := str[i]
		if ch >= 0x80 {
			buf = append(buf, '%')
			buf = strconv.AppendUint(buf, uint64(ch), 16)
		} else {
			buf = append(buf, ch)
		}
	}
	return string(buf)
}

func purgeContentHeaders(hdrs http.Header) {
	for key := range hdrs {
		lc := strings.ToLower(key)
		switch {
		case lc == "etag":
			delete(hdrs, key)
		case lc == "last-modified":
			delete(hdrs, key)
		case strings.HasPrefix(lc, "content-"):
			delete(hdrs, key)
		}
	}
}

func getErrorPagePath(impl *Impl, statusCode int) string {
	statusCodeAsStr := fmt.Sprintf("%03d", statusCode)
	relativePath, found := impl.cfg.ErrorPages.Map[statusCodeAsStr]
	if !found {
		relativePath = statusCodeAsStr + ".tmpl"
	}
	return filepath.Join(impl.cfg.ErrorPages.Root, relativePath)
}

func tryWriteErrorPage(bw *BasicWriter, impl *Impl, statusCode int, u *url.URL, fullPath string) bool {
	contents, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return false
	}

	return tryWriteErrorPage2(bw, impl, statusCode, u, string(contents))
}

func tryWriteErrorPage2(bw *BasicWriter, impl *Impl, statusCode int, u *url.URL, contents string) bool {
	t := htmltemplate.New("page")
	t, err := t.Parse(string(contents))
	if err != nil {
		return false
	}

	data := struct {
		StatusCode int
		StatusText string
		StatusLine string
		URL        *url.URL
	}{
		StatusCode: statusCode,
		StatusText: http.StatusText(statusCode),
		StatusLine: fmt.Sprintf("%03d %s", statusCode, http.StatusText(statusCode)),
		URL:        u,
	}

	var buf bytes.Buffer
	buf.Grow(len(contents))
	err = t.Execute(&buf, data)
	if err != nil {
		return false
	}
	rendered := []byte(buf.String())

	var (
		contentType string
		contentLang string
	)
	if impl != nil && impl.cfg.ErrorPages != nil {
		contentType = impl.cfg.ErrorPages.ContentType
		contentLang = impl.cfg.ErrorPages.ContentLang
	}
	if contentType == "" {
		contentType = defaultErrorPageType
	}
	if contentLang == "" {
		contentLang = defaultErrorPageLang
	}

	hdrs := bw.Header()
	hdrs.Set("content-type", contentType)
	hdrs.Set("content-language", contentLang)
	hdrs.Set("content-length", strconv.FormatUint(uint64(len(rendered)), 10))
	bw.WriteHeader(statusCode)
	bw.Write(rendered)
	return true
}
