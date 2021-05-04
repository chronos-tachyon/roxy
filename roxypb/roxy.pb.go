// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.6.1
// source: roxypb/roxy.proto

package roxypb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Event_Type int32

const (
	Event_UNKNOWN            Event_Type = 0
	Event_INSERT_IP          Event_Type = 1
	Event_DELETE_IP          Event_Type = 2
	Event_UPDATE_WEIGHT      Event_Type = 3
	Event_NEW_SERVICE_CONFIG Event_Type = 4
)

// Enum value maps for Event_Type.
var (
	Event_Type_name = map[int32]string{
		0: "UNKNOWN",
		1: "INSERT_IP",
		2: "DELETE_IP",
		3: "UPDATE_WEIGHT",
		4: "NEW_SERVICE_CONFIG",
	}
	Event_Type_value = map[string]int32{
		"UNKNOWN":            0,
		"INSERT_IP":          1,
		"DELETE_IP":          2,
		"UPDATE_WEIGHT":      3,
		"NEW_SERVICE_CONFIG": 4,
	}
)

func (x Event_Type) Enum() *Event_Type {
	p := new(Event_Type)
	*p = x
	return p
}

func (x Event_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Event_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_roxypb_roxy_proto_enumTypes[0].Descriptor()
}

func (Event_Type) Type() protoreflect.EnumType {
	return &file_roxypb_roxy_proto_enumTypes[0]
}

func (x Event_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Event_Type.Descriptor instead.
func (Event_Type) EnumDescriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{6, 0}
}

type WebMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BodyChunk []byte      `protobuf:"bytes,1,opt,name=body_chunk,json=bodyChunk,proto3" json:"body_chunk,omitempty"`
	Headers   []*KeyValue `protobuf:"bytes,2,rep,name=headers,proto3" json:"headers,omitempty"`
	Trailers  []*KeyValue `protobuf:"bytes,3,rep,name=trailers,proto3" json:"trailers,omitempty"`
}

func (x *WebMessage) Reset() {
	*x = WebMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *WebMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*WebMessage) ProtoMessage() {}

func (x *WebMessage) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use WebMessage.ProtoReflect.Descriptor instead.
func (*WebMessage) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{0}
}

func (x *WebMessage) GetBodyChunk() []byte {
	if x != nil {
		return x.BodyChunk
	}
	return nil
}

func (x *WebMessage) GetHeaders() []*KeyValue {
	if x != nil {
		return x.Headers
	}
	return nil
}

func (x *WebMessage) GetTrailers() []*KeyValue {
	if x != nil {
		return x.Trailers
	}
	return nil
}

type KeyValue struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Key   string `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Value string `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
}

func (x *KeyValue) Reset() {
	*x = KeyValue{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KeyValue) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KeyValue) ProtoMessage() {}

func (x *KeyValue) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KeyValue.ProtoReflect.Descriptor instead.
func (*KeyValue) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{1}
}

func (x *KeyValue) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

func (x *KeyValue) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

type ReportRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Load        float32                    `protobuf:"fixed32,1,opt,name=load,proto3" json:"load,omitempty"`
	FirstReport *ReportRequest_FirstReport `protobuf:"bytes,2,opt,name=first_report,json=firstReport,proto3" json:"first_report,omitempty"`
}

func (x *ReportRequest) Reset() {
	*x = ReportRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReportRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportRequest) ProtoMessage() {}

func (x *ReportRequest) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportRequest.ProtoReflect.Descriptor instead.
func (*ReportRequest) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{2}
}

func (x *ReportRequest) GetLoad() float32 {
	if x != nil {
		return x.Load
	}
	return 0
}

func (x *ReportRequest) GetFirstReport() *ReportRequest_FirstReport {
	if x != nil {
		return x.FirstReport
	}
	return nil
}

type ReportResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ReportResponse) Reset() {
	*x = ReportResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReportResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportResponse) ProtoMessage() {}

func (x *ReportResponse) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportResponse.ProtoReflect.Descriptor instead.
func (*ReportResponse) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{3}
}

type BalanceRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name     string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Location string `protobuf:"bytes,2,opt,name=location,proto3" json:"location,omitempty"`
}

func (x *BalanceRequest) Reset() {
	*x = BalanceRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BalanceRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BalanceRequest) ProtoMessage() {}

func (x *BalanceRequest) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BalanceRequest.ProtoReflect.Descriptor instead.
func (*BalanceRequest) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{4}
}

func (x *BalanceRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *BalanceRequest) GetLocation() string {
	if x != nil {
		return x.Location
	}
	return ""
}

type BalanceResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Events    []*Event `protobuf:"bytes,1,rep,name=events,proto3" json:"events,omitempty"`
	Name      string   `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	IsSharded bool     `protobuf:"varint,3,opt,name=is_sharded,json=isSharded,proto3" json:"is_sharded,omitempty"`
}

func (x *BalanceResponse) Reset() {
	*x = BalanceResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BalanceResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BalanceResponse) ProtoMessage() {}

func (x *BalanceResponse) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BalanceResponse.ProtoReflect.Descriptor instead.
func (*BalanceResponse) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{5}
}

func (x *BalanceResponse) GetEvents() []*Event {
	if x != nil {
		return x.Events
	}
	return nil
}

func (x *BalanceResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *BalanceResponse) GetIsSharded() bool {
	if x != nil {
		return x.IsSharded
	}
	return false
}

type Event struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	EventType         Event_Type `protobuf:"varint,1,opt,name=event_type,json=eventType,proto3,enum=roxy.Event_Type" json:"event_type,omitempty"`
	Unique            string     `protobuf:"bytes,2,opt,name=unique,proto3" json:"unique,omitempty"`
	Location          string     `protobuf:"bytes,3,opt,name=location,proto3" json:"location,omitempty"`
	ServerName        string     `protobuf:"bytes,4,opt,name=server_name,json=serverName,proto3" json:"server_name,omitempty"`
	ShardId           int32      `protobuf:"varint,5,opt,name=shard_id,json=shardId,proto3" json:"shard_id,omitempty"`
	Ip                []byte     `protobuf:"bytes,6,opt,name=ip,proto3" json:"ip,omitempty"`
	Zone              string     `protobuf:"bytes,7,opt,name=zone,proto3" json:"zone,omitempty"`
	Port              uint32     `protobuf:"varint,8,opt,name=port,proto3" json:"port,omitempty"`
	Weight            float32    `protobuf:"fixed32,9,opt,name=weight,proto3" json:"weight,omitempty"`
	ServiceConfigJson string     `protobuf:"bytes,64,opt,name=service_config_json,json=serviceConfigJson,proto3" json:"service_config_json,omitempty"`
}

func (x *Event) Reset() {
	*x = Event{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Event) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Event) ProtoMessage() {}

func (x *Event) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Event.ProtoReflect.Descriptor instead.
func (*Event) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{6}
}

func (x *Event) GetEventType() Event_Type {
	if x != nil {
		return x.EventType
	}
	return Event_UNKNOWN
}

func (x *Event) GetUnique() string {
	if x != nil {
		return x.Unique
	}
	return ""
}

func (x *Event) GetLocation() string {
	if x != nil {
		return x.Location
	}
	return ""
}

func (x *Event) GetServerName() string {
	if x != nil {
		return x.ServerName
	}
	return ""
}

func (x *Event) GetShardId() int32 {
	if x != nil {
		return x.ShardId
	}
	return 0
}

func (x *Event) GetIp() []byte {
	if x != nil {
		return x.Ip
	}
	return nil
}

func (x *Event) GetZone() string {
	if x != nil {
		return x.Zone
	}
	return ""
}

func (x *Event) GetPort() uint32 {
	if x != nil {
		return x.Port
	}
	return 0
}

func (x *Event) GetWeight() float32 {
	if x != nil {
		return x.Weight
	}
	return 0
}

func (x *Event) GetServiceConfigJson() string {
	if x != nil {
		return x.ServiceConfigJson
	}
	return ""
}

type PingRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *PingRequest) Reset() {
	*x = PingRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PingRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PingRequest) ProtoMessage() {}

func (x *PingRequest) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PingRequest.ProtoReflect.Descriptor instead.
func (*PingRequest) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{7}
}

type PingResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *PingResponse) Reset() {
	*x = PingResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PingResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PingResponse) ProtoMessage() {}

func (x *PingResponse) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PingResponse.ProtoReflect.Descriptor instead.
func (*PingResponse) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{8}
}

type ReloadRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ReloadRequest) Reset() {
	*x = ReloadRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReloadRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReloadRequest) ProtoMessage() {}

func (x *ReloadRequest) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReloadRequest.ProtoReflect.Descriptor instead.
func (*ReloadRequest) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{9}
}

type ReloadResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ReloadResponse) Reset() {
	*x = ReloadResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[10]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReloadResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReloadResponse) ProtoMessage() {}

func (x *ReloadResponse) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[10]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReloadResponse.ProtoReflect.Descriptor instead.
func (*ReloadResponse) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{10}
}

type ShutdownRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ShutdownRequest) Reset() {
	*x = ShutdownRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[11]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ShutdownRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ShutdownRequest) ProtoMessage() {}

func (x *ShutdownRequest) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[11]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ShutdownRequest.ProtoReflect.Descriptor instead.
func (*ShutdownRequest) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{11}
}

type ShutdownResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ShutdownResponse) Reset() {
	*x = ShutdownResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[12]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ShutdownResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ShutdownResponse) ProtoMessage() {}

func (x *ShutdownResponse) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[12]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ShutdownResponse.ProtoReflect.Descriptor instead.
func (*ShutdownResponse) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{12}
}

type ReportRequest_FirstReport struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name       string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Unique     string `protobuf:"bytes,2,opt,name=unique,proto3" json:"unique,omitempty"`
	Location   string `protobuf:"bytes,3,opt,name=location,proto3" json:"location,omitempty"`
	ServerName string `protobuf:"bytes,4,opt,name=server_name,json=serverName,proto3" json:"server_name,omitempty"`
	ShardId    int32  `protobuf:"varint,5,opt,name=shard_id,json=shardId,proto3" json:"shard_id,omitempty"`
	Ip         []byte `protobuf:"bytes,6,opt,name=ip,proto3" json:"ip,omitempty"`
	Zone       string `protobuf:"bytes,7,opt,name=zone,proto3" json:"zone,omitempty"`
	Port       uint32 `protobuf:"varint,8,opt,name=port,proto3" json:"port,omitempty"`
	HasShardId bool   `protobuf:"varint,16,opt,name=has_shard_id,json=hasShardId,proto3" json:"has_shard_id,omitempty"`
}

func (x *ReportRequest_FirstReport) Reset() {
	*x = ReportRequest_FirstReport{}
	if protoimpl.UnsafeEnabled {
		mi := &file_roxypb_roxy_proto_msgTypes[13]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReportRequest_FirstReport) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportRequest_FirstReport) ProtoMessage() {}

func (x *ReportRequest_FirstReport) ProtoReflect() protoreflect.Message {
	mi := &file_roxypb_roxy_proto_msgTypes[13]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportRequest_FirstReport.ProtoReflect.Descriptor instead.
func (*ReportRequest_FirstReport) Descriptor() ([]byte, []int) {
	return file_roxypb_roxy_proto_rawDescGZIP(), []int{2, 0}
}

func (x *ReportRequest_FirstReport) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *ReportRequest_FirstReport) GetUnique() string {
	if x != nil {
		return x.Unique
	}
	return ""
}

func (x *ReportRequest_FirstReport) GetLocation() string {
	if x != nil {
		return x.Location
	}
	return ""
}

func (x *ReportRequest_FirstReport) GetServerName() string {
	if x != nil {
		return x.ServerName
	}
	return ""
}

func (x *ReportRequest_FirstReport) GetShardId() int32 {
	if x != nil {
		return x.ShardId
	}
	return 0
}

func (x *ReportRequest_FirstReport) GetIp() []byte {
	if x != nil {
		return x.Ip
	}
	return nil
}

func (x *ReportRequest_FirstReport) GetZone() string {
	if x != nil {
		return x.Zone
	}
	return ""
}

func (x *ReportRequest_FirstReport) GetPort() uint32 {
	if x != nil {
		return x.Port
	}
	return 0
}

func (x *ReportRequest_FirstReport) GetHasShardId() bool {
	if x != nil {
		return x.HasShardId
	}
	return false
}

var File_roxypb_roxy_proto protoreflect.FileDescriptor

var file_roxypb_roxy_proto_rawDesc = []byte{
	0x0a, 0x11, 0x72, 0x6f, 0x78, 0x79, 0x70, 0x62, 0x2f, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x12, 0x04, 0x72, 0x6f, 0x78, 0x79, 0x22, 0x81, 0x01, 0x0a, 0x0a, 0x57, 0x65,
	0x62, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x62, 0x6f, 0x64, 0x79,
	0x5f, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x62, 0x6f,
	0x64, 0x79, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x12, 0x28, 0x0a, 0x07, 0x68, 0x65, 0x61, 0x64, 0x65,
	0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e,
	0x4b, 0x65, 0x79, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x52, 0x07, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72,
	0x73, 0x12, 0x2a, 0x0a, 0x08, 0x74, 0x72, 0x61, 0x69, 0x6c, 0x65, 0x72, 0x73, 0x18, 0x03, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x4b, 0x65, 0x79, 0x56, 0x61,
	0x6c, 0x75, 0x65, 0x52, 0x08, 0x74, 0x72, 0x61, 0x69, 0x6c, 0x65, 0x72, 0x73, 0x22, 0x32, 0x0a,
	0x08, 0x4b, 0x65, 0x79, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x22, 0xd5, 0x02, 0x0a, 0x0d, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6c, 0x6f, 0x61, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x02, 0x52, 0x04, 0x6c, 0x6f, 0x61, 0x64, 0x12, 0x42, 0x0a, 0x0c, 0x66, 0x69, 0x72, 0x73, 0x74,
	0x5f, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1f, 0x2e,
	0x72, 0x6f, 0x78, 0x79, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x2e, 0x46, 0x69, 0x72, 0x73, 0x74, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x0b,
	0x66, 0x69, 0x72, 0x73, 0x74, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x1a, 0xeb, 0x01, 0x0a, 0x0b,
	0x46, 0x69, 0x72, 0x73, 0x74, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12,
	0x16, 0x0a, 0x06, 0x75, 0x6e, 0x69, 0x71, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x06, 0x75, 0x6e, 0x69, 0x71, 0x75, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x5f, 0x6e, 0x61,
	0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x4e, 0x61, 0x6d, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x73, 0x68, 0x61, 0x72, 0x64, 0x5f, 0x69, 0x64,
	0x18, 0x05, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x73, 0x68, 0x61, 0x72, 0x64, 0x49, 0x64, 0x12,
	0x0e, 0x0a, 0x02, 0x69, 0x70, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x02, 0x69, 0x70, 0x12,
	0x12, 0x0a, 0x04, 0x7a, 0x6f, 0x6e, 0x65, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x7a,
	0x6f, 0x6e, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x18, 0x08, 0x20, 0x01, 0x28,
	0x0d, 0x52, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x20, 0x0a, 0x0c, 0x68, 0x61, 0x73, 0x5f, 0x73,
	0x68, 0x61, 0x72, 0x64, 0x5f, 0x69, 0x64, 0x18, 0x10, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a, 0x68,
	0x61, 0x73, 0x53, 0x68, 0x61, 0x72, 0x64, 0x49, 0x64, 0x22, 0x10, 0x0a, 0x0e, 0x52, 0x65, 0x70,
	0x6f, 0x72, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x40, 0x0a, 0x0e, 0x42,
	0x61, 0x6c, 0x61, 0x6e, 0x63, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d,
	0x65, 0x12, 0x1a, 0x0a, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x69, 0x0a,
	0x0f, 0x42, 0x61, 0x6c, 0x61, 0x6e, 0x63, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x23, 0x0a, 0x06, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b,
	0x32, 0x0b, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x52, 0x06, 0x65,
	0x76, 0x65, 0x6e, 0x74, 0x73, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x69, 0x73, 0x5f,
	0x73, 0x68, 0x61, 0x72, 0x64, 0x65, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x69,
	0x73, 0x53, 0x68, 0x61, 0x72, 0x64, 0x65, 0x64, 0x22, 0x86, 0x03, 0x0a, 0x05, 0x45, 0x76, 0x65,
	0x6e, 0x74, 0x12, 0x2f, 0x0a, 0x0a, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x5f, 0x74, 0x79, 0x70, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x10, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x45, 0x76,
	0x65, 0x6e, 0x74, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x09, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x54,
	0x79, 0x70, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x75, 0x6e, 0x69, 0x71, 0x75, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x06, 0x75, 0x6e, 0x69, 0x71, 0x75, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x6c,
	0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6c,
	0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x73, 0x68, 0x61, 0x72,
	0x64, 0x5f, 0x69, 0x64, 0x18, 0x05, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x73, 0x68, 0x61, 0x72,
	0x64, 0x49, 0x64, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x70, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x02, 0x69, 0x70, 0x12, 0x12, 0x0a, 0x04, 0x7a, 0x6f, 0x6e, 0x65, 0x18, 0x07, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x7a, 0x6f, 0x6e, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x18,
	0x08, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x77,
	0x65, 0x69, 0x67, 0x68, 0x74, 0x18, 0x09, 0x20, 0x01, 0x28, 0x02, 0x52, 0x06, 0x77, 0x65, 0x69,
	0x67, 0x68, 0x74, 0x12, 0x2e, 0x0a, 0x13, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x63,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x5f, 0x6a, 0x73, 0x6f, 0x6e, 0x18, 0x40, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x11, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x4a,
	0x73, 0x6f, 0x6e, 0x22, 0x5c, 0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0b, 0x0a, 0x07, 0x55,
	0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x10, 0x00, 0x12, 0x0d, 0x0a, 0x09, 0x49, 0x4e, 0x53, 0x45,
	0x52, 0x54, 0x5f, 0x49, 0x50, 0x10, 0x01, 0x12, 0x0d, 0x0a, 0x09, 0x44, 0x45, 0x4c, 0x45, 0x54,
	0x45, 0x5f, 0x49, 0x50, 0x10, 0x02, 0x12, 0x11, 0x0a, 0x0d, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45,
	0x5f, 0x57, 0x45, 0x49, 0x47, 0x48, 0x54, 0x10, 0x03, 0x12, 0x16, 0x0a, 0x12, 0x4e, 0x45, 0x57,
	0x5f, 0x53, 0x45, 0x52, 0x56, 0x49, 0x43, 0x45, 0x5f, 0x43, 0x4f, 0x4e, 0x46, 0x49, 0x47, 0x10,
	0x04, 0x22, 0x0d, 0x0a, 0x0b, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x22, 0x0e, 0x0a, 0x0c, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x0f, 0x0a, 0x0d, 0x52, 0x65, 0x6c, 0x6f, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x22, 0x10, 0x0a, 0x0e, 0x52, 0x65, 0x6c, 0x6f, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x11, 0x0a, 0x0f, 0x53, 0x68, 0x75, 0x74, 0x64, 0x6f, 0x77, 0x6e, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x12, 0x0a, 0x10, 0x53, 0x68, 0x75, 0x74, 0x64, 0x6f,
	0x77, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x32, 0x3e, 0x0a, 0x09, 0x57, 0x65,
	0x62, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x12, 0x31, 0x0a, 0x05, 0x53, 0x65, 0x72, 0x76, 0x65,
	0x12, 0x10, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x57, 0x65, 0x62, 0x4d, 0x65, 0x73, 0x73, 0x61,
	0x67, 0x65, 0x1a, 0x10, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x57, 0x65, 0x62, 0x4d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x22, 0x00, 0x28, 0x01, 0x30, 0x01, 0x32, 0x8a, 0x01, 0x0a, 0x11, 0x41,
	0x69, 0x72, 0x54, 0x72, 0x61, 0x66, 0x66, 0x69, 0x63, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x6f, 0x6c,
	0x12, 0x37, 0x0a, 0x06, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x13, 0x2e, 0x72, 0x6f, 0x78,
	0x79, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x14, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x28, 0x01, 0x12, 0x3c, 0x0a, 0x07, 0x42, 0x61, 0x6c,
	0x61, 0x6e, 0x63, 0x65, 0x12, 0x14, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x42, 0x61, 0x6c, 0x61,
	0x6e, 0x63, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x15, 0x2e, 0x72, 0x6f, 0x78,
	0x79, 0x2e, 0x42, 0x61, 0x6c, 0x61, 0x6e, 0x63, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x00, 0x28, 0x01, 0x30, 0x01, 0x32, 0xac, 0x01, 0x0a, 0x05, 0x41, 0x64, 0x6d, 0x69,
	0x6e, 0x12, 0x2f, 0x0a, 0x04, 0x50, 0x69, 0x6e, 0x67, 0x12, 0x11, 0x2e, 0x72, 0x6f, 0x78, 0x79,
	0x2e, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x72,
	0x6f, 0x78, 0x79, 0x2e, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x00, 0x12, 0x35, 0x0a, 0x06, 0x52, 0x65, 0x6c, 0x6f, 0x61, 0x64, 0x12, 0x13, 0x2e, 0x72,
	0x6f, 0x78, 0x79, 0x2e, 0x52, 0x65, 0x6c, 0x6f, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x14, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x52, 0x65, 0x6c, 0x6f, 0x61, 0x64, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x3b, 0x0a, 0x08, 0x53, 0x68, 0x75,
	0x74, 0x64, 0x6f, 0x77, 0x6e, 0x12, 0x15, 0x2e, 0x72, 0x6f, 0x78, 0x79, 0x2e, 0x53, 0x68, 0x75,
	0x74, 0x64, 0x6f, 0x77, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e, 0x72,
	0x6f, 0x78, 0x79, 0x2e, 0x53, 0x68, 0x75, 0x74, 0x64, 0x6f, 0x77, 0x6e, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x28, 0x5a, 0x26, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x63, 0x68, 0x72, 0x6f, 0x6e, 0x6f, 0x73, 0x2d, 0x74, 0x61, 0x63,
	0x68, 0x79, 0x6f, 0x6e, 0x2f, 0x72, 0x6f, 0x78, 0x79, 0x2f, 0x72, 0x6f, 0x78, 0x79, 0x70, 0x62,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_roxypb_roxy_proto_rawDescOnce sync.Once
	file_roxypb_roxy_proto_rawDescData = file_roxypb_roxy_proto_rawDesc
)

func file_roxypb_roxy_proto_rawDescGZIP() []byte {
	file_roxypb_roxy_proto_rawDescOnce.Do(func() {
		file_roxypb_roxy_proto_rawDescData = protoimpl.X.CompressGZIP(file_roxypb_roxy_proto_rawDescData)
	})
	return file_roxypb_roxy_proto_rawDescData
}

var file_roxypb_roxy_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_roxypb_roxy_proto_msgTypes = make([]protoimpl.MessageInfo, 14)
var file_roxypb_roxy_proto_goTypes = []interface{}{
	(Event_Type)(0),                   // 0: roxy.Event.Type
	(*WebMessage)(nil),                // 1: roxy.WebMessage
	(*KeyValue)(nil),                  // 2: roxy.KeyValue
	(*ReportRequest)(nil),             // 3: roxy.ReportRequest
	(*ReportResponse)(nil),            // 4: roxy.ReportResponse
	(*BalanceRequest)(nil),            // 5: roxy.BalanceRequest
	(*BalanceResponse)(nil),           // 6: roxy.BalanceResponse
	(*Event)(nil),                     // 7: roxy.Event
	(*PingRequest)(nil),               // 8: roxy.PingRequest
	(*PingResponse)(nil),              // 9: roxy.PingResponse
	(*ReloadRequest)(nil),             // 10: roxy.ReloadRequest
	(*ReloadResponse)(nil),            // 11: roxy.ReloadResponse
	(*ShutdownRequest)(nil),           // 12: roxy.ShutdownRequest
	(*ShutdownResponse)(nil),          // 13: roxy.ShutdownResponse
	(*ReportRequest_FirstReport)(nil), // 14: roxy.ReportRequest.FirstReport
}
var file_roxypb_roxy_proto_depIdxs = []int32{
	2,  // 0: roxy.WebMessage.headers:type_name -> roxy.KeyValue
	2,  // 1: roxy.WebMessage.trailers:type_name -> roxy.KeyValue
	14, // 2: roxy.ReportRequest.first_report:type_name -> roxy.ReportRequest.FirstReport
	7,  // 3: roxy.BalanceResponse.events:type_name -> roxy.Event
	0,  // 4: roxy.Event.event_type:type_name -> roxy.Event.Type
	1,  // 5: roxy.WebServer.Serve:input_type -> roxy.WebMessage
	3,  // 6: roxy.AirTrafficControl.Report:input_type -> roxy.ReportRequest
	5,  // 7: roxy.AirTrafficControl.Balance:input_type -> roxy.BalanceRequest
	8,  // 8: roxy.Admin.Ping:input_type -> roxy.PingRequest
	10, // 9: roxy.Admin.Reload:input_type -> roxy.ReloadRequest
	12, // 10: roxy.Admin.Shutdown:input_type -> roxy.ShutdownRequest
	1,  // 11: roxy.WebServer.Serve:output_type -> roxy.WebMessage
	4,  // 12: roxy.AirTrafficControl.Report:output_type -> roxy.ReportResponse
	6,  // 13: roxy.AirTrafficControl.Balance:output_type -> roxy.BalanceResponse
	9,  // 14: roxy.Admin.Ping:output_type -> roxy.PingResponse
	11, // 15: roxy.Admin.Reload:output_type -> roxy.ReloadResponse
	13, // 16: roxy.Admin.Shutdown:output_type -> roxy.ShutdownResponse
	11, // [11:17] is the sub-list for method output_type
	5,  // [5:11] is the sub-list for method input_type
	5,  // [5:5] is the sub-list for extension type_name
	5,  // [5:5] is the sub-list for extension extendee
	0,  // [0:5] is the sub-list for field type_name
}

func init() { file_roxypb_roxy_proto_init() }
func file_roxypb_roxy_proto_init() {
	if File_roxypb_roxy_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_roxypb_roxy_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*WebMessage); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KeyValue); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReportRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReportResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BalanceRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BalanceResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Event); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PingRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PingResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReloadRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[10].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReloadResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[11].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ShutdownRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[12].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ShutdownResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_roxypb_roxy_proto_msgTypes[13].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReportRequest_FirstReport); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_roxypb_roxy_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   14,
			NumExtensions: 0,
			NumServices:   3,
		},
		GoTypes:           file_roxypb_roxy_proto_goTypes,
		DependencyIndexes: file_roxypb_roxy_proto_depIdxs,
		EnumInfos:         file_roxypb_roxy_proto_enumTypes,
		MessageInfos:      file_roxypb_roxy_proto_msgTypes,
	}.Build()
	File_roxypb_roxy_proto = out.File
	file_roxypb_roxy_proto_rawDesc = nil
	file_roxypb_roxy_proto_goTypes = nil
	file_roxypb_roxy_proto_depIdxs = nil
}