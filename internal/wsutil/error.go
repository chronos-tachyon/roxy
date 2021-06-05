package wsutil

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gorilla/websocket"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// ConvertErrorToStatusError converts an error (such as a CloseError) into a
// gRPC Status Error.
func ConvertErrorToStatusError(err error) error {
	if err == nil {
		return nil
	}

	var err0 CloseError
	if errors.As(err, &err0) {
		if err0.Code == websocket.CloseNormalClosure {
			return nil
		}
		return err0.GRPCStatus().Err()
	}

	var err1 interface {
		GRPCStatus() *status.Status
	}
	if errors.As(err, &err1) {
		return err1.GRPCStatus().Err()
	}

	return status.Error(codes.Unknown, err.Error())
}

// CloseError represents a WebSocket connection close event.
type CloseError struct {
	Code uint16
	Text string
}

// CloseErrorFromBytes parses a CloseError from the payload bytes of a
// WebSocket CLOSE control message.
func CloseErrorFromBytes(p []byte) CloseError {
	err := CloseError{Code: websocket.CloseAbnormalClosure}
	if len(p) >= 2 {
		err.Code = binary.BigEndian.Uint16(p[0:2])
		err.Text = string(p[2:])
	}
	return err
}

// Error fulfills the error interface.
func (err CloseError) Error() string {
	code := fmt.Sprintf("%04d", err.Code)
	text := err.Text
	if text == "" {
		text = closeErrorDefaultText[err.Code]
	}
	var buf strings.Builder
	buf.Grow(22 + len(text))
	buf.WriteString("WebSocket close error ")
	buf.WriteString(code)
	if text != "" {
		buf.WriteString(": ")
		buf.WriteString(text)
	}
	return buf.String()
}

// AsBytes renders this CloseError as bytes suitable for a CLOSE control
// message's payload.
func (err CloseError) AsBytes() []byte {
	p := make([]byte, 2, 2+len(err.Text))
	binary.BigEndian.PutUint16(p, err.Code)
	p = append(p, []byte(err.Text)...)
	return p
}

// GRPCStatus returns a gRPC Status which is roughly equivalent to this
// CloseError.  The details of the original CloseError are stored in the
// Details field of the Status proto message, as type
// "roxy.v0.WebSocketCloseError".
func (err CloseError) GRPCStatus() *status.Status {
	code, ok := closeErrorGRPCCode[err.Code]
	if !ok {
		code = codes.Unknown
	}

	msg := &roxy_v0.WebSocketCloseError{
		Code: uint32(err.Code),
		Text: err.Text,
	}

	var anyList []*anypb.Any
	if any, e := anypb.New(msg); e == nil {
		anyList = make([]*anypb.Any, 1)
		anyList[0] = any
	}

	return status.FromProto(&spb.Status{
		Code:    int32(code),
		Message: err.Error(),
		Details: anyList,
	})
}

// Is returns true if this CloseError is equivalent to the given error.
//
// Currently this method returns true in only two cases:
//
//   1. If this error has code 1000 (normal closure) and other is io.EOF.
//
//   2. If other is a gRPC Status Error with roxy_v0.WebSocketCloseError in its
//      Details, and that roxy_v0.WebSocketCloseError has identical code and
//      text to this error.
//
func (err CloseError) Is(other error) bool {
	if err.Code == websocket.CloseNormalClosure && other == io.EOF {
		return true
	}

	type grpcStatusError interface {
		error
		GRPCStatus() *status.Status
	}
	if err0, ok := other.(grpcStatusError); ok {
		spb := err0.GRPCStatus().Proto()
		for _, any := range spb.Details {
			var err1 roxy_v0.WebSocketCloseError
			if err2 := any.UnmarshalTo(&err1); err2 == nil {
				return uint32(err.Code) == err1.Code && err.Text == err1.Text
			}
		}
	}

	return false
}

var _ error = CloseError{}

var closeErrorDefaultText = map[uint16]string{
	websocket.CloseNormalClosure:           "goodbye",
	websocket.CloseGoingAway:               "going away",
	websocket.CloseProtocolError:           "protocol error",
	websocket.CloseUnsupportedData:         "unsupported data",
	websocket.CloseNoStatusReceived:        "no status received",
	websocket.CloseAbnormalClosure:         "abnormal close",
	websocket.CloseInvalidFramePayloadData: "invalid frame payload data",
	websocket.ClosePolicyViolation:         "policy violation",
	websocket.CloseMessageTooBig:           "message too big",
	websocket.CloseMandatoryExtension:      "missing a mandatory extension",
	websocket.CloseInternalServerErr:       "internal server error",
	websocket.CloseServiceRestart:          "service is restarting",
	websocket.CloseTryAgainLater:           "try again layer",
	websocket.CloseTLSHandshake:            "TLS handshake error",
}

var closeErrorGRPCCode = map[uint16]codes.Code{
	websocket.CloseNormalClosure:           codes.OK,
	websocket.CloseGoingAway:               codes.Unavailable,
	websocket.CloseProtocolError:           codes.Unknown,
	websocket.CloseUnsupportedData:         codes.InvalidArgument,
	websocket.CloseNoStatusReceived:        codes.Unknown,
	websocket.CloseAbnormalClosure:         codes.Unknown,
	websocket.CloseInvalidFramePayloadData: codes.InvalidArgument,
	websocket.ClosePolicyViolation:         codes.InvalidArgument,
	websocket.CloseMessageTooBig:           codes.InvalidArgument,
	websocket.CloseMandatoryExtension:      codes.FailedPrecondition,
	websocket.CloseInternalServerErr:       codes.Unknown,
	websocket.CloseServiceRestart:          codes.Unavailable,
	websocket.CloseTryAgainLater:           codes.Unavailable,
	websocket.CloseTLSHandshake:            codes.Unknown,
}
