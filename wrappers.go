package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// type WrappedReader {{{

type WrappedReader interface {
	io.ReadCloser
	WasClosed() bool
	BytesRead() int64
}

func WrapReader(next io.ReadCloser) WrappedReader {
	return &basicWrappedReader{next: next}
}

// type basicWrappedReader {{{

type basicWrappedReader struct {
	next     io.ReadCloser
	sawError bool
	closed   bool
	bytes    int64
}

func (wr *basicWrappedReader) Read(p []byte) (int, error) {
	if wr.closed {
		return 0, fs.ErrClosed
	}
	n, err := wr.next.Read(p)
	wr.bytes += int64(n)
	if err != nil && err != io.EOF {
		wr.sawError = true
	}
	return n, err
}

func (wr *basicWrappedReader) Close() error {
	if wr.closed {
		return fs.ErrClosed
	}
	wr.closed = true
	err := wr.next.Close()
	if err != nil {
		wr.sawError = true
	}
	return err
}

func (wr *basicWrappedReader) WasClosed() bool {
	return wr.closed
}

func (wr *basicWrappedReader) BytesRead() int64 {
	return wr.bytes
}

var _ WrappedReader = (*basicWrappedReader)(nil)

// }}}

// }}}

// type WrappedWriter {{{

type WrappedWriter interface {
	http.ResponseWriter
	http.Pusher
	Unwrap() http.ResponseWriter
	Status() int
	BytesWritten() int64
	SetRules(rules []*Rule)
	WriteRedirect(statusCode int, urlstr string)
	WriteError(statusCode int)
	SawError() bool
}

func WrapWriter(impl *Impl, w http.ResponseWriter, r *http.Request) WrappedWriter {
	type fancyInterface interface {
		http.ResponseWriter
		http.Flusher
		http.CloseNotifier
		http.Hijacker
		io.ReaderFrom
	}

	isHEAD := (r.Method == http.MethodHead)

	if _, ok := w.(fancyInterface); ok {
		return &fancyWrappedWriter{basicWrappedWriter{impl: impl, next: w, request: r, isHEAD: isHEAD}}
	}

	type flushInterface interface {
		http.ResponseWriter
		http.Flusher
	}

	if _, ok := w.(flushInterface); ok {
		return &flushWrappedWriter{basicWrappedWriter{impl: impl, next: w, request: r, isHEAD: isHEAD}}
	}

	return &basicWrappedWriter{impl: impl, next: w, request: r, isHEAD: isHEAD}
}

// type basicWrappedWriter {{{

type basicWrappedWriter struct {
	impl        *Impl
	next        http.ResponseWriter
	request     *http.Request
	isHEAD      bool
	wroteHeader bool
	sawError    bool
	status      int
	bytes       int64
	rules       []*Rule
}

func (bw *basicWrappedWriter) noResponseBody() bool {
	return bw.isHEAD || bw.status == http.StatusNoContent
}

func (bw *basicWrappedWriter) Header() http.Header {
	return bw.next.Header()
}

func (bw *basicWrappedWriter) WriteHeader(status int) {
	if bw.wroteHeader {
		return
	}

	bw.status = status
	bw.wroteHeader = true
	for _, rule := range bw.rules {
		rule.ApplyPost(bw, bw.request)
	}
	bw.next.WriteHeader(status)
}

func (bw *basicWrappedWriter) Write(buf []byte) (int, error) {
	bw.WriteHeader(http.StatusOK)
	if bw.noResponseBody() {
		return len(buf), nil
	}
	n, err := bw.next.Write(buf)
	bw.bytes += int64(n)
	return n, err
}

func (bw *basicWrappedWriter) Push(url string, opts *http.PushOptions) error {
	if x, ok := bw.next.(http.Pusher); ok {
		return x.Push(url, opts)
	}
	return http.ErrNotSupported
}

func (bw *basicWrappedWriter) Unwrap() http.ResponseWriter {
	return bw.next
}

func (bw *basicWrappedWriter) Status() int {
	return bw.status
}

func (bw *basicWrappedWriter) BytesWritten() int64 {
	return bw.bytes
}

func (bw *basicWrappedWriter) SetRules(rules []*Rule) {
	bw.rules = rules
}

func (bw *basicWrappedWriter) WriteRedirect(statusCode int, urlstr string) {
	if statusCode < 300 || statusCode >= 400 {
		panic(fmt.Errorf("HTTP status code %03d out of range [300..399]", statusCode))
	}

	if bw.wroteHeader {
		bw.sawError = true
		return
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

	if bw.request.Method == http.MethodHead || bw.request.Method == http.MethodGet {
		key := fmt.Sprintf("%03d", statusCode)
		page, found := bw.impl.pages[key]
		if !found {
			page, found = bw.impl.pages["redir"]
		}
		if !found {
			panic(fmt.Errorf("impl.pages[%q] not found", "redir"))
		}

		writeErrorPage(bw, page, statusCode, u)
		return
	}

	hdrs.Set("content-length", "0")
	bw.WriteHeader(statusCode)
}

func (bw *basicWrappedWriter) WriteError(statusCode int) {
	if statusCode < 400 || statusCode >= 600 {
		panic(fmt.Errorf("HTTP status code %03d out of range [400..599]", statusCode))
	}

	if statusCode >= 500 {
		bw.sawError = true
	}

	if bw.wroteHeader {
		bw.sawError = true
		return
	}

	hdrs := bw.Header()
	purgeContentHeaders(hdrs)
	hdrs.Set("cache-control", "no-cache")

	key := fmt.Sprintf("%03d", statusCode)
	page, found := bw.impl.pages[key]
	if !found {
		key2 := "4xx"
		if statusCode >= 500 {
			key2 = "5xx"
		}
		page, found = bw.impl.pages[key2]
	}
	if !found {
		page, found = bw.impl.pages["error"]
	}
	if !found {
		panic(fmt.Errorf("impl.pages[%q] not found", "error"))
	}

	writeErrorPage(bw, page, statusCode, nil)
}

func (bw *basicWrappedWriter) SawError() bool {
	return bw.sawError
}

var _ WrappedWriter = (*basicWrappedWriter)(nil)

// }}}

// type flushWrappedWriter {{{

type flushWrappedWriter struct {
	basicWrappedWriter
}

func (fw *flushWrappedWriter) Flush() {
	fw.basicWrappedWriter.WriteHeader(http.StatusOK)
	fw.basicWrappedWriter.next.(http.Flusher).Flush()
}

var (
	_ WrappedWriter = (*flushWrappedWriter)(nil)
	_ http.Flusher  = (*flushWrappedWriter)(nil)
)

// }}}

// type fancyWrappedWriter {{{

type fancyWrappedWriter struct {
	basicWrappedWriter
}

func (fw *fancyWrappedWriter) CloseNotify() <-chan bool {
	return fw.basicWrappedWriter.next.(http.CloseNotifier).CloseNotify()
}

func (fw *fancyWrappedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return fw.basicWrappedWriter.next.(http.Hijacker).Hijack()
}

func (fw *fancyWrappedWriter) Flush() {
	fw.basicWrappedWriter.WriteHeader(http.StatusOK)
	fw.basicWrappedWriter.next.(http.Flusher).Flush()
}

func (fw *fancyWrappedWriter) ReadFrom(r io.Reader) (int64, error) {
	fw.basicWrappedWriter.WriteHeader(http.StatusOK)
	n, err := fw.basicWrappedWriter.next.(io.ReaderFrom).ReadFrom(r)
	fw.basicWrappedWriter.bytes += n
	return n, err
}

var (
	_ WrappedWriter      = (*fancyWrappedWriter)(nil)
	_ http.CloseNotifier = (*fancyWrappedWriter)(nil)
	_ http.Hijacker      = (*fancyWrappedWriter)(nil)
	_ http.Flusher       = (*fancyWrappedWriter)(nil)
	_ io.ReaderFrom      = (*fancyWrappedWriter)(nil)
)

// }}}

// }}}

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
		case lc == "digest":
			delete(hdrs, key)
		case lc == "etag":
			delete(hdrs, key)
		case lc == "last-modified":
			delete(hdrs, key)
		case strings.HasPrefix(lc, "content-"):
			delete(hdrs, key)
		}
	}
}

func writeErrorPage(bw *basicWrappedWriter, page pageData, statusCode int, u *url.URL) {
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
	buf.Grow(page.size)
	if err := page.tmpl.Execute(&buf, data); err != nil {
		panic(err)
	}
	rendered := buf.Bytes()

	hdrs := bw.Header()
	hdrs.Set("content-type", page.contentType)
	if page.contentLang != "" {
		hdrs.Set("content-language", page.contentLang)
	}
	if page.contentEnc != "" {
		hdrs.Set("content-encoding", page.contentEnc)
	}
	hdrs.Set("content-length", strconv.Itoa(len(rendered)))
	bw.WriteHeader(statusCode)
	_, _ = bw.Write(rendered)
}
