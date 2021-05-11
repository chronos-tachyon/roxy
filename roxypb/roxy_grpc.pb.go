// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package roxypb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// WebServerClient is the client API for WebServer service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type WebServerClient interface {
	Serve(ctx context.Context, opts ...grpc.CallOption) (WebServer_ServeClient, error)
}

type webServerClient struct {
	cc grpc.ClientConnInterface
}

func NewWebServerClient(cc grpc.ClientConnInterface) WebServerClient {
	return &webServerClient{cc}
}

func (c *webServerClient) Serve(ctx context.Context, opts ...grpc.CallOption) (WebServer_ServeClient, error) {
	stream, err := c.cc.NewStream(ctx, &WebServer_ServiceDesc.Streams[0], "/roxy.WebServer/Serve", opts...)
	if err != nil {
		return nil, err
	}
	x := &webServerServeClient{stream}
	return x, nil
}

type WebServer_ServeClient interface {
	Send(*WebMessage) error
	Recv() (*WebMessage, error)
	grpc.ClientStream
}

type webServerServeClient struct {
	grpc.ClientStream
}

func (x *webServerServeClient) Send(m *WebMessage) error {
	return x.ClientStream.SendMsg(m)
}

func (x *webServerServeClient) Recv() (*WebMessage, error) {
	m := new(WebMessage)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// WebServerServer is the server API for WebServer service.
// All implementations must embed UnimplementedWebServerServer
// for forward compatibility
type WebServerServer interface {
	Serve(WebServer_ServeServer) error
	mustEmbedUnimplementedWebServerServer()
}

// UnimplementedWebServerServer must be embedded to have forward compatible implementations.
type UnimplementedWebServerServer struct {
}

func (UnimplementedWebServerServer) Serve(WebServer_ServeServer) error {
	return status.Errorf(codes.Unimplemented, "method Serve not implemented")
}
func (UnimplementedWebServerServer) mustEmbedUnimplementedWebServerServer() {}

// UnsafeWebServerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to WebServerServer will
// result in compilation errors.
type UnsafeWebServerServer interface {
	mustEmbedUnimplementedWebServerServer()
}

func RegisterWebServerServer(s grpc.ServiceRegistrar, srv WebServerServer) {
	s.RegisterService(&WebServer_ServiceDesc, srv)
}

func _WebServer_Serve_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(WebServerServer).Serve(&webServerServeServer{stream})
}

type WebServer_ServeServer interface {
	Send(*WebMessage) error
	Recv() (*WebMessage, error)
	grpc.ServerStream
}

type webServerServeServer struct {
	grpc.ServerStream
}

func (x *webServerServeServer) Send(m *WebMessage) error {
	return x.ServerStream.SendMsg(m)
}

func (x *webServerServeServer) Recv() (*WebMessage, error) {
	m := new(WebMessage)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// WebServer_ServiceDesc is the grpc.ServiceDesc for WebServer service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var WebServer_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "roxy.WebServer",
	HandlerType: (*WebServerServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Serve",
			Handler:       _WebServer_Serve_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "roxypb/roxy.proto",
}

// AirTrafficControlClient is the client API for AirTrafficControl service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type AirTrafficControlClient interface {
	Lookup(ctx context.Context, in *LookupRequest, opts ...grpc.CallOption) (*LookupResponse, error)
	Find(ctx context.Context, in *FindRequest, opts ...grpc.CallOption) (*FindResponse, error)
	ServerAnnounce(ctx context.Context, opts ...grpc.CallOption) (AirTrafficControl_ServerAnnounceClient, error)
	ClientAssign(ctx context.Context, opts ...grpc.CallOption) (AirTrafficControl_ClientAssignClient, error)
}

type airTrafficControlClient struct {
	cc grpc.ClientConnInterface
}

func NewAirTrafficControlClient(cc grpc.ClientConnInterface) AirTrafficControlClient {
	return &airTrafficControlClient{cc}
}

func (c *airTrafficControlClient) Lookup(ctx context.Context, in *LookupRequest, opts ...grpc.CallOption) (*LookupResponse, error) {
	out := new(LookupResponse)
	err := c.cc.Invoke(ctx, "/roxy.AirTrafficControl/Lookup", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *airTrafficControlClient) Find(ctx context.Context, in *FindRequest, opts ...grpc.CallOption) (*FindResponse, error) {
	out := new(FindResponse)
	err := c.cc.Invoke(ctx, "/roxy.AirTrafficControl/Find", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *airTrafficControlClient) ServerAnnounce(ctx context.Context, opts ...grpc.CallOption) (AirTrafficControl_ServerAnnounceClient, error) {
	stream, err := c.cc.NewStream(ctx, &AirTrafficControl_ServiceDesc.Streams[0], "/roxy.AirTrafficControl/ServerAnnounce", opts...)
	if err != nil {
		return nil, err
	}
	x := &airTrafficControlServerAnnounceClient{stream}
	return x, nil
}

type AirTrafficControl_ServerAnnounceClient interface {
	Send(*ServerAnnounceRequest) error
	Recv() (*ServerAnnounceResponse, error)
	grpc.ClientStream
}

type airTrafficControlServerAnnounceClient struct {
	grpc.ClientStream
}

func (x *airTrafficControlServerAnnounceClient) Send(m *ServerAnnounceRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *airTrafficControlServerAnnounceClient) Recv() (*ServerAnnounceResponse, error) {
	m := new(ServerAnnounceResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *airTrafficControlClient) ClientAssign(ctx context.Context, opts ...grpc.CallOption) (AirTrafficControl_ClientAssignClient, error) {
	stream, err := c.cc.NewStream(ctx, &AirTrafficControl_ServiceDesc.Streams[1], "/roxy.AirTrafficControl/ClientAssign", opts...)
	if err != nil {
		return nil, err
	}
	x := &airTrafficControlClientAssignClient{stream}
	return x, nil
}

type AirTrafficControl_ClientAssignClient interface {
	Send(*ClientAssignRequest) error
	Recv() (*ClientAssignResponse, error)
	grpc.ClientStream
}

type airTrafficControlClientAssignClient struct {
	grpc.ClientStream
}

func (x *airTrafficControlClientAssignClient) Send(m *ClientAssignRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *airTrafficControlClientAssignClient) Recv() (*ClientAssignResponse, error) {
	m := new(ClientAssignResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// AirTrafficControlServer is the server API for AirTrafficControl service.
// All implementations must embed UnimplementedAirTrafficControlServer
// for forward compatibility
type AirTrafficControlServer interface {
	Lookup(context.Context, *LookupRequest) (*LookupResponse, error)
	Find(context.Context, *FindRequest) (*FindResponse, error)
	ServerAnnounce(AirTrafficControl_ServerAnnounceServer) error
	ClientAssign(AirTrafficControl_ClientAssignServer) error
	mustEmbedUnimplementedAirTrafficControlServer()
}

// UnimplementedAirTrafficControlServer must be embedded to have forward compatible implementations.
type UnimplementedAirTrafficControlServer struct {
}

func (UnimplementedAirTrafficControlServer) Lookup(context.Context, *LookupRequest) (*LookupResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Lookup not implemented")
}
func (UnimplementedAirTrafficControlServer) Find(context.Context, *FindRequest) (*FindResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Find not implemented")
}
func (UnimplementedAirTrafficControlServer) ServerAnnounce(AirTrafficControl_ServerAnnounceServer) error {
	return status.Errorf(codes.Unimplemented, "method ServerAnnounce not implemented")
}
func (UnimplementedAirTrafficControlServer) ClientAssign(AirTrafficControl_ClientAssignServer) error {
	return status.Errorf(codes.Unimplemented, "method ClientAssign not implemented")
}
func (UnimplementedAirTrafficControlServer) mustEmbedUnimplementedAirTrafficControlServer() {}

// UnsafeAirTrafficControlServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AirTrafficControlServer will
// result in compilation errors.
type UnsafeAirTrafficControlServer interface {
	mustEmbedUnimplementedAirTrafficControlServer()
}

func RegisterAirTrafficControlServer(s grpc.ServiceRegistrar, srv AirTrafficControlServer) {
	s.RegisterService(&AirTrafficControl_ServiceDesc, srv)
}

func _AirTrafficControl_Lookup_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(LookupRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AirTrafficControlServer).Lookup(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/roxy.AirTrafficControl/Lookup",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AirTrafficControlServer).Lookup(ctx, req.(*LookupRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AirTrafficControl_Find_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FindRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AirTrafficControlServer).Find(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/roxy.AirTrafficControl/Find",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AirTrafficControlServer).Find(ctx, req.(*FindRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AirTrafficControl_ServerAnnounce_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(AirTrafficControlServer).ServerAnnounce(&airTrafficControlServerAnnounceServer{stream})
}

type AirTrafficControl_ServerAnnounceServer interface {
	Send(*ServerAnnounceResponse) error
	Recv() (*ServerAnnounceRequest, error)
	grpc.ServerStream
}

type airTrafficControlServerAnnounceServer struct {
	grpc.ServerStream
}

func (x *airTrafficControlServerAnnounceServer) Send(m *ServerAnnounceResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *airTrafficControlServerAnnounceServer) Recv() (*ServerAnnounceRequest, error) {
	m := new(ServerAnnounceRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _AirTrafficControl_ClientAssign_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(AirTrafficControlServer).ClientAssign(&airTrafficControlClientAssignServer{stream})
}

type AirTrafficControl_ClientAssignServer interface {
	Send(*ClientAssignResponse) error
	Recv() (*ClientAssignRequest, error)
	grpc.ServerStream
}

type airTrafficControlClientAssignServer struct {
	grpc.ServerStream
}

func (x *airTrafficControlClientAssignServer) Send(m *ClientAssignResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *airTrafficControlClientAssignServer) Recv() (*ClientAssignRequest, error) {
	m := new(ClientAssignRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// AirTrafficControl_ServiceDesc is the grpc.ServiceDesc for AirTrafficControl service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var AirTrafficControl_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "roxy.AirTrafficControl",
	HandlerType: (*AirTrafficControlServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Lookup",
			Handler:    _AirTrafficControl_Lookup_Handler,
		},
		{
			MethodName: "Find",
			Handler:    _AirTrafficControl_Find_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ServerAnnounce",
			Handler:       _AirTrafficControl_ServerAnnounce_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "ClientAssign",
			Handler:       _AirTrafficControl_ClientAssign_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "roxypb/roxy.proto",
}

// AdminClient is the client API for Admin service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type AdminClient interface {
	Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (*PingResponse, error)
	Reload(ctx context.Context, in *ReloadRequest, opts ...grpc.CallOption) (*ReloadResponse, error)
	Shutdown(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*ShutdownResponse, error)
}

type adminClient struct {
	cc grpc.ClientConnInterface
}

func NewAdminClient(cc grpc.ClientConnInterface) AdminClient {
	return &adminClient{cc}
}

func (c *adminClient) Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (*PingResponse, error) {
	out := new(PingResponse)
	err := c.cc.Invoke(ctx, "/roxy.Admin/Ping", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminClient) Reload(ctx context.Context, in *ReloadRequest, opts ...grpc.CallOption) (*ReloadResponse, error) {
	out := new(ReloadResponse)
	err := c.cc.Invoke(ctx, "/roxy.Admin/Reload", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminClient) Shutdown(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*ShutdownResponse, error) {
	out := new(ShutdownResponse)
	err := c.cc.Invoke(ctx, "/roxy.Admin/Shutdown", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AdminServer is the server API for Admin service.
// All implementations must embed UnimplementedAdminServer
// for forward compatibility
type AdminServer interface {
	Ping(context.Context, *PingRequest) (*PingResponse, error)
	Reload(context.Context, *ReloadRequest) (*ReloadResponse, error)
	Shutdown(context.Context, *ShutdownRequest) (*ShutdownResponse, error)
	mustEmbedUnimplementedAdminServer()
}

// UnimplementedAdminServer must be embedded to have forward compatible implementations.
type UnimplementedAdminServer struct {
}

func (UnimplementedAdminServer) Ping(context.Context, *PingRequest) (*PingResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Ping not implemented")
}
func (UnimplementedAdminServer) Reload(context.Context, *ReloadRequest) (*ReloadResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Reload not implemented")
}
func (UnimplementedAdminServer) Shutdown(context.Context, *ShutdownRequest) (*ShutdownResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Shutdown not implemented")
}
func (UnimplementedAdminServer) mustEmbedUnimplementedAdminServer() {}

// UnsafeAdminServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AdminServer will
// result in compilation errors.
type UnsafeAdminServer interface {
	mustEmbedUnimplementedAdminServer()
}

func RegisterAdminServer(s grpc.ServiceRegistrar, srv AdminServer) {
	s.RegisterService(&Admin_ServiceDesc, srv)
}

func _Admin_Ping_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PingRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).Ping(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/roxy.Admin/Ping",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).Ping(ctx, req.(*PingRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Admin_Reload_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReloadRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).Reload(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/roxy.Admin/Reload",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).Reload(ctx, req.(*ReloadRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Admin_Shutdown_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ShutdownRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).Shutdown(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/roxy.Admin/Shutdown",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).Shutdown(ctx, req.(*ShutdownRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Admin_ServiceDesc is the grpc.ServiceDesc for Admin service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Admin_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "roxy.Admin",
	HandlerType: (*AdminServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Ping",
			Handler:    _Admin_Ping_Handler,
		},
		{
			MethodName: "Reload",
			Handler:    _Admin_Reload_Handler,
		},
		{
			MethodName: "Shutdown",
			Handler:    _Admin_Shutdown_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "roxypb/roxy.proto",
}
