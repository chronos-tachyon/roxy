package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// type ResponseWriter {{{

type ResponseWriter interface {
	http.ResponseWriter
	http.Pusher
	Unwrap() http.ResponseWriter
	Status() int
	BytesWritten() int64
	SetRules(rules []*Rule, req *http.Request)
}

// type basicWriter {{{

type basicWriter struct {
	next        http.ResponseWriter
	wroteHeader bool
	status      int
	bytes       int64
	rules       []*Rule
	request     *http.Request
}

func (bw *basicWriter) Header() http.Header {
	return bw.next.Header()
}

func (bw *basicWriter) WriteHeader(status int) {
	if !bw.wroteHeader {
		bw.status = status
		bw.wroteHeader = true

		for _, rule := range bw.rules {
			rule.ApplyPost(bw.next, bw.request)
		}

		bw.next.WriteHeader(status)
	}
}

func (bw *basicWriter) Write(buf []byte) (int, error) {
	bw.WriteHeader(http.StatusOK)
	n, err := bw.next.Write(buf)
	bw.bytes += int64(n)
	return n, err
}

func (bw *basicWriter) Push(target string, opts *http.PushOptions) error {
	if x, ok := bw.next.(http.Pusher); ok {
		return x.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (bw *basicWriter) Unwrap() http.ResponseWriter {
	return bw.next
}

func (bw *basicWriter) Status() int {
	return bw.status
}

func (bw *basicWriter) BytesWritten() int64 {
	return bw.bytes
}

func (bw *basicWriter) SetRules(rules []*Rule, req *http.Request) {
	bw.rules = rules
	bw.request = req
}

var (
	_ http.ResponseWriter = (*basicWriter)(nil)
	_ http.Pusher         = (*basicWriter)(nil)
	_ ResponseWriter      = (*basicWriter)(nil)
)

// }}}

// type flushWriter {{{

type flushWriter struct {
	basicWriter
}

func (fw *flushWriter) Flush() {
	fw.basicWriter.next.(http.Flusher).Flush()
}

var (
	_ http.ResponseWriter = (*flushWriter)(nil)
	_ http.Pusher         = (*flushWriter)(nil)
	_ http.Flusher        = (*flushWriter)(nil)
	_ ResponseWriter      = (*flushWriter)(nil)
)

// }}}

// type fancyWriter {{{

type fancyWriter struct {
	basicWriter
}

func (fw *fancyWriter) Flush() {
	fw.basicWriter.next.(http.Flusher).Flush()
}

func (fw *fancyWriter) CloseNotify() <-chan bool {
	return fw.basicWriter.next.(http.CloseNotifier).CloseNotify()
}

func (fw *fancyWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return fw.basicWriter.next.(http.Hijacker).Hijack()
}

func (fw *fancyWriter) ReadFrom(r io.Reader) (int64, error) {
	fw.basicWriter.WriteHeader(http.StatusOK)
	return fw.basicWriter.next.(io.ReaderFrom).ReadFrom(r)
}

var (
	_ http.ResponseWriter = (*fancyWriter)(nil)
	_ http.Pusher         = (*fancyWriter)(nil)
	_ http.Flusher        = (*fancyWriter)(nil)
	_ http.CloseNotifier  = (*fancyWriter)(nil)
	_ http.Hijacker       = (*fancyWriter)(nil)
	_ io.ReaderFrom       = (*fancyWriter)(nil)
	_ ResponseWriter      = (*fancyWriter)(nil)
)

// }}}

// }}}

func WrapWriter(w http.ResponseWriter) ResponseWriter {
	type fancyInterface interface {
		http.ResponseWriter
		http.Flusher
		http.CloseNotifier
		http.Hijacker
		io.ReaderFrom
	}

	if _, ok := w.(fancyInterface); ok {
		return &fancyWriter{basicWriter{next: w}}
	}

	type flushInterface interface {
		http.ResponseWriter
		http.Flusher
	}

	if _, ok := w.(flushInterface); ok {
		return &flushWriter{basicWriter{next: w}}
	}

	return &basicWriter{next: w}
}
