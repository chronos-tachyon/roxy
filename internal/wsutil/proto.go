package wsutil

import (
	"github.com/gorilla/websocket"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

var validMessageTypes = map[int]struct{}{
	websocket.TextMessage:   {},
	websocket.BinaryMessage: {},
	websocket.CloseMessage:  {},
	websocket.PingMessage:   {},
	websocket.PongMessage:   {},
}

// MessageTypeToProto converts a gorilla/websocket message type integer to a
// roxy_v0.WebSocketFrame_Type enum.
func MessageTypeToProto(messageType int) roxy_v0.WebSocketFrame_Type {
	_, ok := validMessageTypes[messageType]
	roxyutil.Assertf(ok, "invalid messageType %d", messageType)
	return roxy_v0.WebSocketFrame_Type(messageType)
}

// MessageTypeFromProto converts a roxy_v0.WebSocketFrame_Type enum to a
// gorilla/websocket message type integer.
func MessageTypeFromProto(t roxy_v0.WebSocketFrame_Type) int {
	messageType := int(t)
	_, ok := validMessageTypes[messageType]
	roxyutil.Assertf(ok, "invalid messageType %d", messageType)
	return messageType
}
