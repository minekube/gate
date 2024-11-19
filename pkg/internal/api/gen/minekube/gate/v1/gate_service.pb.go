// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.2
// 	protoc        (unknown)
// source: minekube/gate/v1/gate_service.proto

package gatev1

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

type RemoveServerRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name    string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *RemoveServerRequest) Reset() {
	*x = RemoveServerRequest{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RemoveServerRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RemoveServerRequest) ProtoMessage() {}

func (x *RemoveServerRequest) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RemoveServerRequest.ProtoReflect.Descriptor instead.
func (*RemoveServerRequest) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{0}
}

func (x *RemoveServerRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *RemoveServerRequest) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

type RemoveServerResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Server *ServerRemoval `protobuf:"bytes,1,opt,name=server,proto3" json:"server,omitempty"`
}

func (x *RemoveServerResponse) Reset() {
	*x = RemoveServerResponse{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RemoveServerResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RemoveServerResponse) ProtoMessage() {}

func (x *RemoveServerResponse) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RemoveServerResponse.ProtoReflect.Descriptor instead.
func (*RemoveServerResponse) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{1}
}

func (x *RemoveServerResponse) GetServer() *ServerRemoval {
	if x != nil {
		return x.Server
	}
	return nil
}

type ServerRemoval struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name    string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *ServerRemoval) Reset() {
	*x = ServerRemoval{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ServerRemoval) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerRemoval) ProtoMessage() {}

func (x *ServerRemoval) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerRemoval.ProtoReflect.Descriptor instead.
func (*ServerRemoval) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{2}
}

func (x *ServerRemoval) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *ServerRemoval) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

type AddServerRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name    string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *AddServerRequest) Reset() {
	*x = AddServerRequest{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *AddServerRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AddServerRequest) ProtoMessage() {}

func (x *AddServerRequest) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AddServerRequest.ProtoReflect.Descriptor instead.
func (*AddServerRequest) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{3}
}

func (x *AddServerRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *AddServerRequest) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

type GetServerResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Server *ServerAddition `protobuf:"bytes,1,opt,name=server,proto3" json:"server,omitempty"`
}

func (x *GetServerResponse) Reset() {
	*x = GetServerResponse{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetServerResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetServerResponse) ProtoMessage() {}

func (x *GetServerResponse) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetServerResponse.ProtoReflect.Descriptor instead.
func (*GetServerResponse) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{4}
}

func (x *GetServerResponse) GetServer() *ServerAddition {
	if x != nil {
		return x.Server
	}
	return nil
}

type ServerAddition struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name    string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *ServerAddition) Reset() {
	*x = ServerAddition{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ServerAddition) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerAddition) ProtoMessage() {}

func (x *ServerAddition) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerAddition.ProtoReflect.Descriptor instead.
func (*ServerAddition) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{5}
}

func (x *ServerAddition) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *ServerAddition) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

// ListServersRequest is the request for ListServers method.
type ListServersRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ListServersRequest) Reset() {
	*x = ListServersRequest{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListServersRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListServersRequest) ProtoMessage() {}

func (x *ListServersRequest) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListServersRequest.ProtoReflect.Descriptor instead.
func (*ListServersRequest) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{6}
}

// ListServersResponse is the response for ListServers method.
type ListServersResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Servers []*Server `protobuf:"bytes,1,rep,name=servers,proto3" json:"servers,omitempty"`
}

func (x *ListServersResponse) Reset() {
	*x = ListServersResponse{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListServersResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListServersResponse) ProtoMessage() {}

func (x *ListServersResponse) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListServersResponse.ProtoReflect.Descriptor instead.
func (*ListServersResponse) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{7}
}

func (x *ListServersResponse) GetServers() []*Server {
	if x != nil {
		return x.Servers
	}
	return nil
}

// Server is a backend server where Gate can connect players to.
type Server struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name    string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
	Players int32  `protobuf:"varint,3,opt,name=players,proto3" json:"players,omitempty"`
}

func (x *Server) Reset() {
	*x = Server{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Server) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Server) ProtoMessage() {}

func (x *Server) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Server.ProtoReflect.Descriptor instead.
func (*Server) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{8}
}

func (x *Server) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Server) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

func (x *Server) GetPlayers() int32 {
	if x != nil {
		return x.Players
	}
	return 0
}

// GetPlayerRequest is the request for GetPlayer method.
type GetPlayerRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Gets the player by the given id (Minecraft UUID).
	// Optional, if not set the username will be used.
	// If both id and username are set, the id will be used.
	//
	// Format but be a valid Minecraft UUID.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// Gets the player by the given username.
	// Optional, if not set the id will be used.
	Username string `protobuf:"bytes,2,opt,name=username,proto3" json:"username,omitempty"`
}

func (x *GetPlayerRequest) Reset() {
	*x = GetPlayerRequest{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[9]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetPlayerRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetPlayerRequest) ProtoMessage() {}

func (x *GetPlayerRequest) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[9]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetPlayerRequest.ProtoReflect.Descriptor instead.
func (*GetPlayerRequest) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{9}
}

func (x *GetPlayerRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *GetPlayerRequest) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

// GetPlayerResponse is the response for GetPlayer method.
type GetPlayerResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The player matching the request.
	Player *Player `protobuf:"bytes,1,opt,name=player,proto3" json:"player,omitempty"`
}

func (x *GetPlayerResponse) Reset() {
	*x = GetPlayerResponse{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[10]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetPlayerResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetPlayerResponse) ProtoMessage() {}

func (x *GetPlayerResponse) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[10]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetPlayerResponse.ProtoReflect.Descriptor instead.
func (*GetPlayerResponse) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{10}
}

func (x *GetPlayerResponse) GetPlayer() *Player {
	if x != nil {
		return x.Player
	}
	return nil
}

// Player is a Gate player.
type Player struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id       string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Username string `protobuf:"bytes,2,opt,name=username,proto3" json:"username,omitempty"`
}

func (x *Player) Reset() {
	*x = Player{}
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[11]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Player) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Player) ProtoMessage() {}

func (x *Player) ProtoReflect() protoreflect.Message {
	mi := &file_minekube_gate_v1_gate_service_proto_msgTypes[11]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Player.ProtoReflect.Descriptor instead.
func (*Player) Descriptor() ([]byte, []int) {
	return file_minekube_gate_v1_gate_service_proto_rawDescGZIP(), []int{11}
}

func (x *Player) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *Player) GetUsername() string {
	if x != nil {
		return x.Username
	}
	return ""
}

var File_minekube_gate_v1_gate_service_proto protoreflect.FileDescriptor

var file_minekube_gate_v1_gate_service_proto_rawDesc = []byte{
	0x0a, 0x23, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x2f,
	0x76, 0x31, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x5f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x10, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e,
	0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x22, 0x43, 0x0a, 0x13, 0x52, 0x65, 0x6d, 0x6f, 0x76,
	0x65, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x22, 0x4f, 0x0a, 0x14,
	0x52, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x37, 0x0a, 0x06, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x1f, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e,
	0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65,
	0x6d, 0x6f, 0x76, 0x61, 0x6c, 0x52, 0x06, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x22, 0x3d, 0x0a,
	0x0d, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x6d, 0x6f, 0x76, 0x61, 0x6c, 0x12, 0x12,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x22, 0x40, 0x0a, 0x10,
	0x41, 0x64, 0x64, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x22, 0x4d,
	0x0a, 0x11, 0x47, 0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x38, 0x0a, 0x06, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x20, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67,
	0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x41, 0x64, 0x64,
	0x69, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x06, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x22, 0x3e, 0x0a,
	0x0e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x41, 0x64, 0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x12,
	0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x22, 0x14, 0x0a,
	0x12, 0x4c, 0x69, 0x73, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x22, 0x49, 0x0a, 0x13, 0x4c, 0x69, 0x73, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x32, 0x0a, 0x07, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x6d, 0x69,
	0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x53,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x07, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x22, 0x50,
	0x0a, 0x06, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07,
	0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61,
	0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72,
	0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x73,
	0x22, 0x3e, 0x0a, 0x10, 0x47, 0x65, 0x74, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x02, 0x69, 0x64, 0x12, 0x1a, 0x0a, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65,
	0x22, 0x45, 0x0a, 0x11, 0x47, 0x65, 0x74, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x30, 0x0a, 0x06, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65,
	0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x52,
	0x06, 0x70, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x22, 0x34, 0x0a, 0x06, 0x50, 0x6c, 0x61, 0x79, 0x65,
	0x72, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69,
	0x64, 0x12, 0x1a, 0x0a, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x08, 0x75, 0x73, 0x65, 0x72, 0x6e, 0x61, 0x6d, 0x65, 0x32, 0xf4, 0x02,
	0x0a, 0x0b, 0x47, 0x61, 0x74, 0x65, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x54, 0x0a,
	0x09, 0x47, 0x65, 0x74, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x12, 0x22, 0x2e, 0x6d, 0x69, 0x6e,
	0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65,
	0x74, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x23,
	0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76,
	0x31, 0x2e, 0x47, 0x65, 0x74, 0x50, 0x6c, 0x61, 0x79, 0x65, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x5a, 0x0a, 0x0b, 0x4c, 0x69, 0x73, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x73, 0x12, 0x24, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61,
	0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x25, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b,
	0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x4c, 0x69, 0x73, 0x74,
	0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x54, 0x0a, 0x09, 0x41, 0x64, 0x64, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x12, 0x22, 0x2e, 0x6d,
	0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e,
	0x41, 0x64, 0x64, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x23, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65,
	0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x5d, 0x0a, 0x0c, 0x52, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x53,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x12, 0x25, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65,
	0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x52, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x53,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x26, 0x2e, 0x6d,
	0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x2e,
	0x52, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x42, 0xcd, 0x01, 0x0a, 0x14, 0x63, 0x6f, 0x6d, 0x2e, 0x6d, 0x69, 0x6e,
	0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x67, 0x61, 0x74, 0x65, 0x2e, 0x76, 0x31, 0x42, 0x10, 0x47,
	0x61, 0x74, 0x65, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50,
	0x01, 0x5a, 0x41, 0x67, 0x6f, 0x2e, 0x6d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x6d, 0x69, 0x6e,
	0x65, 0x6b, 0x75, 0x62, 0x65, 0x2f, 0x67, 0x61, 0x74, 0x65, 0x2f, 0x76, 0x31, 0x3b, 0x67, 0x61,
	0x74, 0x65, 0x76, 0x31, 0xa2, 0x02, 0x03, 0x4d, 0x47, 0x58, 0xaa, 0x02, 0x10, 0x4d, 0x69, 0x6e,
	0x65, 0x6b, 0x75, 0x62, 0x65, 0x2e, 0x47, 0x61, 0x74, 0x65, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x10,
	0x4d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x5c, 0x47, 0x61, 0x74, 0x65, 0x5c, 0x56, 0x31,
	0xe2, 0x02, 0x1c, 0x4d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x5c, 0x47, 0x61, 0x74, 0x65,
	0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea,
	0x02, 0x12, 0x4d, 0x69, 0x6e, 0x65, 0x6b, 0x75, 0x62, 0x65, 0x3a, 0x3a, 0x47, 0x61, 0x74, 0x65,
	0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_minekube_gate_v1_gate_service_proto_rawDescOnce sync.Once
	file_minekube_gate_v1_gate_service_proto_rawDescData = file_minekube_gate_v1_gate_service_proto_rawDesc
)

func file_minekube_gate_v1_gate_service_proto_rawDescGZIP() []byte {
	file_minekube_gate_v1_gate_service_proto_rawDescOnce.Do(func() {
		file_minekube_gate_v1_gate_service_proto_rawDescData = protoimpl.X.CompressGZIP(file_minekube_gate_v1_gate_service_proto_rawDescData)
	})
	return file_minekube_gate_v1_gate_service_proto_rawDescData
}

var file_minekube_gate_v1_gate_service_proto_msgTypes = make([]protoimpl.MessageInfo, 12)
var file_minekube_gate_v1_gate_service_proto_goTypes = []any{
	(*RemoveServerRequest)(nil),  // 0: minekube.gate.v1.RemoveServerRequest
	(*RemoveServerResponse)(nil), // 1: minekube.gate.v1.RemoveServerResponse
	(*ServerRemoval)(nil),        // 2: minekube.gate.v1.ServerRemoval
	(*AddServerRequest)(nil),     // 3: minekube.gate.v1.AddServerRequest
	(*GetServerResponse)(nil),    // 4: minekube.gate.v1.GetServerResponse
	(*ServerAddition)(nil),       // 5: minekube.gate.v1.ServerAddition
	(*ListServersRequest)(nil),   // 6: minekube.gate.v1.ListServersRequest
	(*ListServersResponse)(nil),  // 7: minekube.gate.v1.ListServersResponse
	(*Server)(nil),               // 8: minekube.gate.v1.Server
	(*GetPlayerRequest)(nil),     // 9: minekube.gate.v1.GetPlayerRequest
	(*GetPlayerResponse)(nil),    // 10: minekube.gate.v1.GetPlayerResponse
	(*Player)(nil),               // 11: minekube.gate.v1.Player
}
var file_minekube_gate_v1_gate_service_proto_depIdxs = []int32{
	2,  // 0: minekube.gate.v1.RemoveServerResponse.server:type_name -> minekube.gate.v1.ServerRemoval
	5,  // 1: minekube.gate.v1.GetServerResponse.server:type_name -> minekube.gate.v1.ServerAddition
	8,  // 2: minekube.gate.v1.ListServersResponse.servers:type_name -> minekube.gate.v1.Server
	11, // 3: minekube.gate.v1.GetPlayerResponse.player:type_name -> minekube.gate.v1.Player
	9,  // 4: minekube.gate.v1.GateService.GetPlayer:input_type -> minekube.gate.v1.GetPlayerRequest
	6,  // 5: minekube.gate.v1.GateService.ListServers:input_type -> minekube.gate.v1.ListServersRequest
	3,  // 6: minekube.gate.v1.GateService.AddServer:input_type -> minekube.gate.v1.AddServerRequest
	0,  // 7: minekube.gate.v1.GateService.RemoveServer:input_type -> minekube.gate.v1.RemoveServerRequest
	10, // 8: minekube.gate.v1.GateService.GetPlayer:output_type -> minekube.gate.v1.GetPlayerResponse
	7,  // 9: minekube.gate.v1.GateService.ListServers:output_type -> minekube.gate.v1.ListServersResponse
	4,  // 10: minekube.gate.v1.GateService.AddServer:output_type -> minekube.gate.v1.GetServerResponse
	1,  // 11: minekube.gate.v1.GateService.RemoveServer:output_type -> minekube.gate.v1.RemoveServerResponse
	8,  // [8:12] is the sub-list for method output_type
	4,  // [4:8] is the sub-list for method input_type
	4,  // [4:4] is the sub-list for extension type_name
	4,  // [4:4] is the sub-list for extension extendee
	0,  // [0:4] is the sub-list for field type_name
}

func init() { file_minekube_gate_v1_gate_service_proto_init() }
func file_minekube_gate_v1_gate_service_proto_init() {
	if File_minekube_gate_v1_gate_service_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_minekube_gate_v1_gate_service_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   12,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_minekube_gate_v1_gate_service_proto_goTypes,
		DependencyIndexes: file_minekube_gate_v1_gate_service_proto_depIdxs,
		MessageInfos:      file_minekube_gate_v1_gate_service_proto_msgTypes,
	}.Build()
	File_minekube_gate_v1_gate_service_proto = out.File
	file_minekube_gate_v1_gate_service_proto_rawDesc = nil
	file_minekube_gate_v1_gate_service_proto_goTypes = nil
	file_minekube_gate_v1_gate_service_proto_depIdxs = nil
}
