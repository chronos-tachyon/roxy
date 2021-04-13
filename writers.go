package main

import (
	"bufio"
	"context"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strings"
)

// type WrappedWriter {{{

type WrappedWriter interface {
	http.ResponseWriter
	http.Pusher
	Unwrap() http.ResponseWriter
	Status() int
	BytesWritten() int64
	SetRules(rules []*Rule, req *http.Request)
	Error(ctx context.Context, status int)
	SawError() bool
}

// type BasicWriter {{{

type BasicWriter struct {
	next        http.ResponseWriter
	wroteHeader bool
	sawError    bool
	status      int
	bytes       int64
	rules       []*Rule
	request     *http.Request
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

func (bw *BasicWriter) SetRules(rules []*Rule, req *http.Request) {
	bw.rules = rules
	bw.request = req
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
	hdrs.Set("cache-control", "no-cache")
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

	haveImpl := true
	if value := ctx.Value(implKey{}); value == nil {
		haveImpl = false
	}

	if haveImpl {
		impl := implFromCtx(ctx)
		if impl.cfg.ErrorPages != nil && impl.cfg.ErrorPages.Root != "" {
			rootDir := impl.cfg.ErrorPages.Root
			errorPageMap := impl.cfg.ErrorPages.Map
			fullPath := getErrorPagePath(statusCode, rootDir, errorPageMap)
			if ok := tryWriteErrorPage(bw, impl, statusCode, fullPath); ok {
				return
			}

			if statusCode != 400 && statusCode != 500 {
				altStatusCode := 500
				if statusCode < 500 {
					altStatusCode = 400
				}

				fullPath = getErrorPagePath(altStatusCode, rootDir, errorPageMap)
				if ok := tryWriteErrorPage(bw, impl, statusCode, fullPath); ok {
					return
				}
			}
		}
	}

	// fallback

	statusText := fmt.Sprintf("%03d %s", statusCode, http.StatusText(statusCode))

	var buf strings.Builder
	buf.WriteString("<!DOCTYPE html>\n")
	buf.WriteString("<html>\n")
	buf.WriteString("\t<head>\n")
	buf.WriteString("\t\t<meta charset=\"utf-8\">\n")
	buf.WriteString("\t\t<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	buf.WriteString("\t\t<title>")
	buf.WriteString(html.EscapeString(statusText))
	buf.WriteString("</title>\n")
	buf.WriteString("\t</head>\n")
	buf.WriteString("\t<body>\n")
	buf.WriteString("\t\t<h1>")
	buf.WriteString(html.EscapeString(statusText))
	buf.WriteString("</h1>\n")
	buf.WriteString("\t</body>\n")
	buf.WriteString("</html>\n")
	contents := []byte(buf.String())

	hdrs.Set("content-type", "text/html; charset=utf-8")
	hdrs.Set("content-language", "en")
	bw.WriteHeader(statusCode)
	bw.Write(contents)
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

func getErrorPagePath(statusCode int, rootDir string, errorPageMap map[string]string) string {
	statusCodeAsStr := fmt.Sprintf("%03d", statusCode)
	errorPagePath, found := errorPageMap[statusCodeAsStr]
	if !found {
		errorPagePath = statusCodeAsStr + ".html"
	}
	fullPath := filepath.Join(rootDir, errorPagePath)
	return fullPath
}

func tryWriteErrorPage(bw *BasicWriter, impl *Impl, statusCode int, fullPath string) bool {
	contents, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return false
	}

	contentType := impl.cfg.ErrorPages.ContentType
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}

	contentLang := impl.cfg.ErrorPages.ContentLang
	if contentLang == "" {
		contentLang = "en"
	}

	hdrs := bw.Header()
	hdrs.Set("content-type", contentType)
	hdrs.Set("content-language", contentLang)
	bw.WriteHeader(statusCode)
	bw.Write(contents)
	return true
}
