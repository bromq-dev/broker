package grpc

import (
	"context"

	"google.golang.org/grpc"
)

// RouterServiceClient is the client API for RouterService.
type RouterServiceClient interface {
	Route(ctx context.Context, in *RouteRequest, opts ...grpc.CallOption) (*RouteResponse, error)
}

type routerServiceClient struct {
	cc grpc.ClientConnInterface
}

// NewRouterServiceClient creates a new RouterServiceClient.
func NewRouterServiceClient(cc grpc.ClientConnInterface) RouterServiceClient {
	return &routerServiceClient{cc}
}

func (c *routerServiceClient) Route(ctx context.Context, in *RouteRequest, opts ...grpc.CallOption) (*RouteResponse, error) {
	out := new(RouteResponse)
	err := c.cc.Invoke(ctx, "/cluster.RouterService/Route", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RouterServiceServer is the server API for RouterService.
type RouterServiceServer interface {
	Route(context.Context, *RouteRequest) (*RouteResponse, error)
}

// UnimplementedRouterServiceServer can be embedded for forward compatibility.
type UnimplementedRouterServiceServer struct{}

func (UnimplementedRouterServiceServer) Route(context.Context, *RouteRequest) (*RouteResponse, error) {
	return nil, nil
}

// RegisterRouterServiceServer registers the server.
func RegisterRouterServiceServer(s *grpc.Server, srv RouterServiceServer) {
	s.RegisterService(&_RouterService_serviceDesc, srv)
}

var _RouterService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "cluster.RouterService",
	HandlerType: (*RouterServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Route",
			Handler:    _RouterService_Route_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "router.proto",
}

func _RouterService_Route_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RouteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServiceServer).Route(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/cluster.RouterService/Route",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServiceServer).Route(ctx, req.(*RouteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// routerServer implements RouterServiceServer.
type routerServer struct {
	UnimplementedRouterServiceServer
	router *Router
}

func (s *routerServer) Route(ctx context.Context, req *RouteRequest) (*RouteResponse, error) {
	s.router.handleMessage(req)
	return &RouteResponse{}, nil
}

// Message types

// RouteRequest is the request for routing a message.
type RouteRequest struct {
	Topic   string `protobuf:"bytes,1,opt,name=topic,proto3" json:"topic,omitempty"`
	Payload []byte `protobuf:"bytes,2,opt,name=payload,proto3" json:"payload,omitempty"`
	Qos     uint32 `protobuf:"varint,3,opt,name=qos,proto3" json:"qos,omitempty"`
	Retain  bool   `protobuf:"varint,4,opt,name=retain,proto3" json:"retain,omitempty"`
}

func (m *RouteRequest) Reset()         { *m = RouteRequest{} }
func (m *RouteRequest) String() string { return "" }
func (m *RouteRequest) ProtoMessage()  {}

// RouteResponse is the response for routing.
type RouteResponse struct{}

func (m *RouteResponse) Reset()         { *m = RouteResponse{} }
func (m *RouteResponse) String() string { return "" }
func (m *RouteResponse) ProtoMessage()  {}
