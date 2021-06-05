package wsutil

import (
	"time"

	"github.com/gorilla/websocket"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

const (
	defaultWriteTimeout = 24 * time.Hour
	defaultReadTimeout  = 24 * time.Hour
	defaultReadLimit    = 1 << 20 // 1 MiB
)

type adaptorOptions struct {
	writeTimeout time.Duration
	readTimeout  time.Duration
	readLimit    int64
}

// AdaptorOption represents an option for building an Adaptor.
type AdaptorOption func(*adaptorOptions)

// WithWriteTimeout specifies the connection write timeout for an Adaptor.  See
// the websocket.Conn.SetWriteDeadline method for more information.
func WithWriteTimeout(timeout time.Duration) AdaptorOption {
	return func(o *adaptorOptions) {
		o.writeTimeout = timeout
	}
}

// WithReadTimeout specifies the connection read timeout for an Adaptor.  See
// the websocket.Conn.SetReadDeadline method for more information.
func WithReadTimeout(timeout time.Duration) AdaptorOption {
	return func(o *adaptorOptions) {
		o.readTimeout = timeout
	}
}

// WithReadLimit specifies the maximum number of bytes to read per message for
// an Adaptor.  See the websocket.Conn.SetReadLimit method for more
// information.
func WithReadLimit(limit int64) AdaptorOption {
	return func(o *adaptorOptions) {
		o.readLimit = limit
	}
}

// Adaptor wraps a websocket.Conn so that it can be used as a gRPC-like
// WebSocketConn suitable for use with Looper.
type Adaptor struct {
	conn         *websocket.Conn
	writeTimeout time.Duration
	readTimeout  time.Duration
	readLimit    int64
}

// MakeAdaptor builds and returns an Adaptor.
func MakeAdaptor(conn *websocket.Conn, opts ...AdaptorOption) Adaptor {
	roxyutil.AssertNotNil(&conn)

	o := adaptorOptions{
		writeTimeout: defaultWriteTimeout,
		readTimeout:  defaultReadTimeout,
		readLimit:    defaultReadLimit,
	}

	for _, opt := range opts {
		opt(&o)
	}

	adaptor := Adaptor{
		conn:         conn,
		writeTimeout: o.writeTimeout,
		readTimeout:  o.readTimeout,
		readLimit:    o.readLimit,
	}
	return adaptor
}

// Send fulfills the WebSocketConn interface.
func (adaptor Adaptor) Send(frame *roxy_v0.WebSocketFrame) error {
	messageType := MessageTypeFromProto(frame.FrameType)
	payload := frame.Payload

	_ = adaptor.conn.SetWriteDeadline(time.Now().Add(adaptor.writeTimeout))
	err := adaptor.conn.WriteMessage(messageType, payload)
	return ConvertErrorToStatusError(err)
}

// Recv fulfills the WebSocketConn interface.
func (adaptor Adaptor) Recv() (*roxy_v0.WebSocketFrame, error) {
	adaptor.conn.SetReadLimit(adaptor.readLimit)
	_ = adaptor.conn.SetReadDeadline(time.Now().Add(adaptor.readTimeout))

	messageType, payload, err := adaptor.conn.ReadMessage()
	if err != nil {
		return nil, ConvertErrorToStatusError(err)
	}

	frame := &roxy_v0.WebSocketFrame{
		FrameType: MessageTypeToProto(messageType),
		Payload:   payload,
	}
	return frame, nil
}

var _ WebSocketConn = Adaptor{}
