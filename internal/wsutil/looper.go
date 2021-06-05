package wsutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

const (
	defaultPingInterval = 15 * time.Second
	defaultPongInterval = 30 * time.Second
)

// WebSocketConn is the minimum subset of the roxy_v0.Web_SocketClient and
// roxy_v0.Web_SocketServer gRPC streams which is needed by Looper.
//
// Implementations may optionally implement interface { CloseSend() error } as
// well, in which case CloseSend() will be called immediately after sending a
// CLOSE control message.
//
type WebSocketConn interface {
	Send(*roxy_v0.WebSocketFrame) error
	Recv() (*roxy_v0.WebSocketFrame, error)
}

// TextHandlerFunc is the type for an OnText handler.
type TextHandlerFunc func(context.Context, *Looper, string)

// BinaryHandlerFunc is the type for an OnBinary handler.
type BinaryHandlerFunc func(context.Context, *Looper, []byte)

// CloseHandlerFunc is the type for an OnClose handler.
type CloseHandlerFunc func(context.Context, *Looper, CloseError)

// PingHandlerFunc is the type for an OnPing handler.
type PingHandlerFunc func(context.Context, *Looper, string)

// PongHandlerFunc is the type for an OnPong handler.
type PongHandlerFunc func(context.Context, *Looper, string)

type looperOptions struct {
	pingInterval time.Duration
	pongInterval time.Duration
	onText       TextHandlerFunc
	onBinary     BinaryHandlerFunc
	onClose      CloseHandlerFunc
	onPing       PingHandlerFunc
	onPong       PongHandlerFunc
}

// LooperOption represents an option for constructing a Looper.
type LooperOption func(*looperOptions)

// WithPingInterval specifies the time interval between the receiving of a PONG
// control message and the sending of the next automatic PING control message.
func WithPingInterval(interval time.Duration) LooperOption {
	return func(o *looperOptions) {
		o.pingInterval = interval
	}
}

// WithPongInterval specifies the time interval between the sending of an
// automatic PING control message and abandoning the connection because no PONG
// control message was received in response.
func WithPongInterval(interval time.Duration) LooperOption {
	return func(o *looperOptions) {
		o.pongInterval = interval
	}
}

// OnText specifies a handler for TEXT data messages.
func OnText(handler TextHandlerFunc) LooperOption {
	return func(o *looperOptions) {
		o.onText = handler
	}
}

// OnBinary specifies a handler for BINARY data messages.
func OnBinary(handler BinaryHandlerFunc) LooperOption {
	return func(o *looperOptions) {
		o.onBinary = handler
	}
}

// OnClose specifies a handler for CLOSE control messages.
//
// CLOSE messages will always be automatically reflected to the peer,
// regardless of whether or not an OnClose handler is specified and regardless
// of what actions the OnClose handler takes.
//
func OnClose(handler CloseHandlerFunc) LooperOption {
	return func(o *looperOptions) {
		o.onClose = handler
	}
}

// OnPing specifies a handler for PING control messages.
//
// PING messages will always be automatically reflected to the peer as PONG
// messages, regardless of whether or not an OnPing handler is specified and
// regardless of what actions the OnPing handler takes.
//
func OnPing(handler PingHandlerFunc) LooperOption {
	return func(o *looperOptions) {
		o.onPing = handler
	}
}

// OnPong specifies a handler for PONG control messages.
//
// PONG messages will be automatically processed, regardless of whether or not
// an OnPong handler is specified and regardless of what actions the OnPong
// handler takes.
//
func OnPong(handler PongHandlerFunc) LooperOption {
	return func(o *looperOptions) {
		o.onPong = handler
	}
}

// Looper is a construct for handling the mid-level details of operating a
// WebSocket-over-gRPC endpoint (client or server).
type Looper struct {
	ctx    context.Context
	conn   WebSocketConn
	logger zerolog.Logger

	pingInterval time.Duration
	pongInterval time.Duration

	onText   TextHandlerFunc
	onBinary BinaryHandlerFunc
	onClose  CloseHandlerFunc
	onPing   PingHandlerFunc
	onPong   PongHandlerFunc

	closeCh chan struct{}
	pongCh  chan struct{}
	sendCh  chan *roxy_v0.WebSocketFrame
	errCh   chan error

	wg  sync.WaitGroup
	err error
}

// NewLooper constructs a new Looper for the given WebSocketConn.
func NewLooper(ctx context.Context, conn WebSocketConn, opts ...LooperOption) *Looper {
	roxyutil.AssertNotNil(&ctx)
	roxyutil.AssertNotNil(&conn)

	logger := zerolog.Ctx(ctx)

	o := looperOptions{
		pingInterval: defaultPingInterval,
		pongInterval: defaultPongInterval,
	}
	for _, opt := range opts {
		opt(&o)
	}

	looper := &Looper{
		ctx:    ctx,
		conn:   conn,
		logger: *logger,

		pingInterval: o.pingInterval,
		pongInterval: o.pongInterval,

		onText:   o.onText,
		onBinary: o.onBinary,
		onClose:  o.onClose,
		onPing:   o.onPing,
		onPong:   o.onPong,

		closeCh: make(chan struct{}),
		pongCh:  make(chan struct{}),
		sendCh:  make(chan *roxy_v0.WebSocketFrame),
		errCh:   make(chan error),
	}

	if looper.pingInterval == 0 {
		looper.pingInterval = defaultPingInterval
	}
	if looper.pongInterval == 0 {
		looper.pongInterval = defaultPongInterval
	}

	looper.wg.Add(3)
	go looper.sendThread()
	go looper.recvThread()
	go looper.pingThread()

	return looper
}

// Wait blocks until the WebSocket connection shuts down, then returns the
// error which caused the connection to shut down, or nil if the connection was
// closed cleanly by a CLOSE control message with code 1000.
func (looper *Looper) Wait() error {
	looper.wg.Wait()
	close(looper.errCh)
	close(looper.sendCh)
	close(looper.pongCh)
	err := looper.err
	if xerr, ok := err.(CloseError); ok && xerr.Code == websocket.CloseNormalClosure {
		err = nil
	}
	return err
}

// SendTextMessage sends a TEXT data message.
//
// It's safe to call this from any thread.
//
func (looper *Looper) SendTextMessage(text string) {
	roxyutil.Assert(utf8.ValidString(text), "text is not valid UTF-8")
	looper.logger.Trace().
		Stringer("payload", textPayloadStringer(text)).
		Int("len", len(text)).
		Msg("SendTextMessage")

	looper.sendCh <- &roxy_v0.WebSocketFrame{
		FrameType: roxy_v0.WebSocketFrame_DATA_TEXT,
		Payload:   []byte(text),
	}
}

// SendBinaryMessage sends a BINARY data message.
//
// It's safe to call this from any thread.
//
func (looper *Looper) SendBinaryMessage(data []byte) {
	looper.logger.Trace().
		Stringer("payload", binPayloadStringer(data)).
		Int("len", len(data)).
		Msg("SendBinaryMessage")

	looper.sendCh <- &roxy_v0.WebSocketFrame{
		FrameType: roxy_v0.WebSocketFrame_DATA_BINARY,
		Payload:   data,
	}
}

// SendClose sends a CLOSE control message.
//
// It's safe to call this from any thread.
//
func (looper *Looper) SendClose(code uint16, text string) {
	roxyutil.Assert(utf8.ValidString(text), "text is not valid UTF-8")
	err := CloseError{Code: code, Text: text}
	data := err.AsBytes()
	looper.logger.Trace().
		Stringer("payload", binPayloadStringer(data)).
		Int("len", len(data)).
		Err(err).
		Msg("SendClose")

	looper.sendCh <- &roxy_v0.WebSocketFrame{
		FrameType: roxy_v0.WebSocketFrame_CTRL_CLOSE,
		Payload:   data,
	}
}

// SendPing sends a PING control message.
//
// It's safe to call this from any thread.
//
// You normally don't need to call this yourself.
//
func (looper *Looper) SendPing(text string) {
	roxyutil.Assert(utf8.ValidString(text), "text is not valid UTF-8")
	looper.logger.Trace().
		Stringer("payload", textPayloadStringer(text)).
		Int("len", len(text)).
		Msg("SendPing")

	looper.sendCh <- &roxy_v0.WebSocketFrame{
		FrameType: roxy_v0.WebSocketFrame_CTRL_PING,
		Payload:   []byte(text),
	}
}

// SendPong sends a PONG control message.
//
// It's safe to call this from any thread.
//
// You normally don't need to call this yourself.
//
func (looper *Looper) SendPong(text string) {
	roxyutil.Assert(utf8.ValidString(text), "text is not valid UTF-8")
	looper.logger.Trace().
		Stringer("payload", textPayloadStringer(text)).
		Int("len", len(text)).
		Msg("SendPong")

	looper.sendCh <- &roxy_v0.WebSocketFrame{
		FrameType: roxy_v0.WebSocketFrame_CTRL_PONG,
		Payload:   []byte(text),
	}
}

func (looper *Looper) sendThread() {
	defer looper.wg.Done()

	doIOErr := func(err error) {
		if err == io.EOF {
			err = nil
		}
		looper.err = err
		close(looper.closeCh)
	}

Loop:
	for {
		select {
		case err := <-looper.errCh:
			doIOErr(err)
			break Loop

		case frame := <-looper.sendCh:
			err := looper.conn.Send(frame)
			if err != nil {
				doIOErr(err)
				break Loop
			}

			if frame.FrameType == roxy_v0.WebSocketFrame_CTRL_CLOSE {
				type closeSender interface {
					WebSocketConn
					CloseSend() error
				}

				if xconn, ok := looper.conn.(closeSender); ok {
					looper.logger.Trace().
						Msg("CloseSend")

					err = xconn.CloseSend()
					if err != nil {
						doIOErr(err)
						break Loop
					}
				}

				looper.err = CloseErrorFromBytes(frame.Payload)
				close(looper.closeCh)
				break Loop
			}
		}
	}

	go func() {
		for range looper.errCh {
			// pass
		}
	}()

	go func() {
		for range looper.sendCh {
			// pass
		}
	}()
}

func (looper *Looper) recvThread() {
	defer looper.wg.Done()

	for {
		frame, err := looper.conn.Recv()
		if err != nil {
			looper.errCh <- err
			return
		}

		switch frame.FrameType {
		case roxy_v0.WebSocketFrame_DATA_TEXT:
			text := string(frame.Payload)
			looper.logger.Trace().
				Stringer("payload", textPayloadStringer(text)).
				Int("len", len(text)).
				Msg("Recv DATA_TEXT")

			if !utf8.ValidString(text) {
				looper.SendClose(websocket.CloseInvalidFramePayloadData, "text is not valid UTF-8")
				continue
			}

			if looper.onText != nil {
				looper.onText(looper.ctx, looper, text)
			}

		case roxy_v0.WebSocketFrame_DATA_BINARY:
			data := frame.Payload
			looper.logger.Trace().
				Stringer("payload", binPayloadStringer(data)).
				Int("len", len(data)).
				Msg("Recv DATA_BINARY")
			if looper.onBinary != nil {
				looper.onBinary(looper.ctx, looper, data)
			}

		case roxy_v0.WebSocketFrame_CTRL_CLOSE:
			data := frame.Payload
			err := CloseErrorFromBytes(data)
			looper.logger.Trace().
				Stringer("payload", binPayloadStringer(data)).
				Int("len", len(data)).
				Err(err).
				Msg("Recv CTRL_CLOSE")

			if !utf8.ValidString(err.Text) {
				looper.SendClose(websocket.CloseInvalidFramePayloadData, "text is not valid UTF-8")
				continue
			}

			looper.SendClose(err.Code, err.Text)
			if looper.onClose != nil {
				looper.onClose(looper.ctx, looper, err)
			}

		case roxy_v0.WebSocketFrame_CTRL_PING:
			text := string(frame.Payload)
			looper.logger.Trace().
				Stringer("payload", textPayloadStringer(text)).
				Int("len", len(text)).
				Msg("Recv CTRL_PING")

			if !utf8.ValidString(text) {
				looper.SendClose(websocket.CloseInvalidFramePayloadData, "text is not valid UTF-8")
				continue
			}

			looper.SendPong(text)
			if looper.onPing != nil {
				looper.onPing(looper.ctx, looper, text)
			}

		case roxy_v0.WebSocketFrame_CTRL_PONG:
			text := string(frame.Payload)
			looper.logger.Trace().
				Stringer("payload", textPayloadStringer(text)).
				Int("len", len(text)).
				Msg("Recv CTRL_PONG")

			if !utf8.ValidString(text) {
				looper.SendClose(websocket.CloseInvalidFramePayloadData, "text is not valid UTF-8")
				continue
			}

			looper.pongCh <- struct{}{}
			if looper.onPong != nil {
				looper.onPong(looper.ctx, looper, text)
			}

		default:
			looper.logger.Error().
				Uint16("frameType", uint16(frame.FrameType)).
				Stringer("payload", binPayloadStringer(frame.Payload)).
				Int("len", len(frame.Payload)).
				Msg("Recv UNKNOWN")
			looper.SendClose(websocket.CloseProtocolError, "unknown frame type")
		}
	}
}

func (looper *Looper) pingThread() {
	defer looper.wg.Done()

	for {
		t := time.NewTimer(looper.pingInterval)
		select {
		case <-looper.pongCh:
			t.Stop()
			continue

		case <-looper.closeCh:
			t.Stop()
			return

		case <-t.C:
			looper.SendPing("")
		}

		t.Reset(looper.pongInterval)
		select {
		case <-looper.pongCh:
			t.Stop()
			continue

		case <-looper.closeCh:
			t.Stop()
			return

		case <-t.C:
			looper.SendClose(websocket.CloseGoingAway, "PONG not received")
			return
		}
	}
}

type textPayloadStringer string

func (p textPayloadStringer) String() string {
	var runeLen int
	runes := make([]rune, 0, 16)
	for _, ch := range string(p) {
		runeLen++
		if len(runes) < cap(runes) {
			runes = append(runes, ch)
		}
	}

	var byteLen int
	for _, ch := range runes {
		byteLen += utf8.RuneLen(ch)
	}

	truncated := (runeLen > len(runes))
	if truncated {
		byteLen += 3
	}

	var buf strings.Builder
	buf.Grow(byteLen)
	for _, ch := range runes {
		buf.WriteRune(ch)
	}
	if truncated {
		buf.WriteString("...")
	}
	return buf.String()
}

var _ fmt.Stringer = textPayloadStringer("")

type binPayloadStringer []byte

func (p binPayloadStringer) String() string {
	n := len(p)
	truncated := false
	if n > 8 {
		n = 8
		truncated = true
	}
	var buf strings.Builder
	buf.Grow(19)
	buf.WriteString(hex.EncodeToString(p[:n]))
	if truncated {
		buf.WriteString("...")
	}
	return buf.String()
}

var _ fmt.Stringer = binPayloadStringer(nil)
