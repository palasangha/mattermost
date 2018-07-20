// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

//go:generate go run interface_generator/main.go

package plugin

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"reflect"

	"github.com/hashicorp/go-plugin"
	"github.com/mattermost/mattermost-server/mlog"
	"github.com/mattermost/mattermost-server/mmproto"
	"google.golang.org/grpc"
)

type hooksGRPCClient struct {
	client      mmproto.HooksClient
	log         *mlog.Logger
	grpcBroker  *plugin.GRPCBroker
	apiImpl     API
	implemented [TotalHooksId]bool
}

type hooksGRPCServer struct {
	impl          interface{}
	grpcBroker    *plugin.GRPCBroker
	apiGRPCClient *apiGRPCClient
}

type apiGRPCClient struct {
	client mmproto.APIClient
	log    *mlog.Logger
}

type apiGRPCServer struct {
	impl API
}

//
// Below are specal cases for hooks or APIs that can not be auto generated
//

func (g *hooksGRPCClient) Implemented() (impl []string, err error) {
	var resp *mmproto.Hooks_ImplementedResponse
	resp, err = g.client.Hooks_Implemented(context.Background(), &mmproto.Hooks_ImplementedRequest{})
	impl = resp.Result1
	for _, hookName := range impl {
		if hookId, ok := hookNameToId[hookName]; ok {
			g.implemented[hookId] = true
		}
	}
	return
}

// Implemented replies with the names of the hooks that are implemented.
func (s *hooksGRPCServer) Implemented() (*mmproto.Hooks_ImplementedResponse, error) {
	ifaceType := reflect.TypeOf((*Hooks)(nil)).Elem()
	implType := reflect.TypeOf(s.impl)
	selfType := reflect.TypeOf(s)
	var methods []string
	for i := 0; i < ifaceType.NumMethod(); i++ {
		method := ifaceType.Method(i)
		if m, ok := implType.MethodByName(method.Name); !ok {
			continue
		} else if m.Type.NumIn() != method.Type.NumIn()+1 {
			continue
		} else if m.Type.NumOut() != method.Type.NumOut() {
			continue
		} else {
			match := true
			for j := 0; j < method.Type.NumIn(); j++ {
				if m.Type.In(j+1) != method.Type.In(j) {
					match = false
					break
				}
			}
			for j := 0; j < method.Type.NumOut(); j++ {
				if m.Type.Out(j) != method.Type.Out(j) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		if _, ok := selfType.MethodByName(method.Name); !ok {
			continue
		}
		methods = append(methods, method.Name)
	}
	return &mmproto.Hooks_ImplementedResponse{Result1: methods}, nil
}

func (g *hooksGRPCClient) OnActivate() error {
	brokerId := g.grpcBroker.NextId()

	apiServer := &apiGRPCServer{
		impl: g.apiImpl,
	}

	var s *grpc.Server
	serverFunc := func(opts []grpc.ServerOption) *grpc.Server {
		s = grpc.NewServer(opts...)
		mmproto.RegisterAPIServer(s, apiServer)
		return s
	}

	go g.grpcBroker.AcceptAndServe(brokerId, serverFunc)

	_, err := g.client.Hooks_OnActivate(context.Background(), &mmproto.Hooks_OnActivateRequest{Arg1: brokerId})
	return err
}

func (s *hooksGRPCServer) OnActivate(ctx context.Context, req *mmproto.Hooks_OnActivateRequest) error {
	connection, err := s.grpcBroker.Dial(req.Arg1)
	if err != nil {
		return err
	}

	s.apiGRPCClient = &apiGRPCClient{
		client: mmproto.NewAPIClient(connection),
	}

	if mmplugin, ok := s.impl.(interface {
		SetAPI(api API)
		OnConfigurationChange() error
	}); !ok {
	} else {
		mmplugin.SetAPI(s.apiGRPCClient)
		mmplugin.OnConfigurationChange()
	}

	// Capture output of standard logger because go-plugin
	// redirects it.
	log.SetOutput(os.Stderr)

	if hook, ok := s.impl.(interface {
		OnActivate() error
	}); ok {
		return hook.OnActivate()
	}
	return nil
}

func (g *apiGRPCClient) LoadPluginConfiguration(dest interface{}) error {
	// TODO: Implement
	return nil
}

func (s *apiGRPCServer) LoadPluginConfiguration(ctx context.Context) error {
	// TODO: Implement
	return nil
}

type Z_ServeHTTPArgs struct {
	ResponseWriterStream uint32
	Request              *http.Request
	Context              *Context
	RequestBodyStream    uint32
}

func (g *hooksGRPCClient) ServeHTTP(c *Context, w http.ResponseWriter, r *http.Request) {
	if !g.implemented[ServeHTTPId] {
		http.NotFound(w, r)
		return
	}

	serveHTTPStreamId := g.muxBroker.NextId()
	go func() {
		connection, err := g.muxBroker.Accept(serveHTTPStreamId)
		if err != nil {
			g.log.Error("Plugin failed to ServeHTTP, muxBroker couldn't accept connection", mlog.Uint32("serve_http_stream_id", serveHTTPStreamId), mlog.Err(err))
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}
		defer connection.Close()

		rpcServer := rpc.NewServer()
		if err := rpcServer.RegisterName("Plugin", &httpResponseWriterRPCServer{w: w}); err != nil {
			g.log.Error("Plugin failed to ServeHTTP, coulden't register RPC name", mlog.Err(err))
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}
		rpcServer.ServeConn(connection)
	}()

	requestBodyStreamId := uint32(0)
	if r.Body != nil {
		requestBodyStreamId = g.muxBroker.NextId()
		go func() {
			bodyConnection, err := g.muxBroker.Accept(requestBodyStreamId)
			if err != nil {
				g.log.Error("Plugin failed to ServeHTTP, muxBroker couldn't Accept request body connecion", mlog.Err(err))
				http.Error(w, "500 internal server error", http.StatusInternalServerError)
				return
			}
			defer bodyConnection.Close()
			serveIOReader(r.Body, bodyConnection)
		}()
	}

	forwardedRequest := &http.Request{
		Method:     r.Method,
		URL:        r.URL,
		Proto:      r.Proto,
		ProtoMajor: r.ProtoMajor,
		ProtoMinor: r.ProtoMinor,
		Header:     r.Header,
		Host:       r.Host,
		RemoteAddr: r.RemoteAddr,
		RequestURI: r.RequestURI,
	}

	if err := g.client.Call("Plugin.ServeHTTP", Z_ServeHTTPArgs{
		Context:              c,
		ResponseWriterStream: serveHTTPStreamId,
		Request:              forwardedRequest,
		RequestBodyStream:    requestBodyStreamId,
	}, nil); err != nil {
		g.log.Error("Plugin failed to ServeHTTP, RPC call failed", mlog.Err(err))
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
	}
	return
}

func (s *hooksGRPCServer) ServeHTTP(args *Z_ServeHTTPArgs, returns *struct{}) error {
	connection, err := s.muxBroker.Dial(args.ResponseWriterStream)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Can't connect to remote response writer stream, error: %v", err.Error())
		return err
	}
	w := connectHTTPResponseWriter(connection)
	defer w.Close()

	r := args.Request
	if args.RequestBodyStream != 0 {
		connection, err := s.muxBroker.Dial(args.RequestBodyStream)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Can't connect to remote request body stream, error: %v", err.Error())
			return err
		}
		r.Body = connectIOReader(connection)
	} else {
		r.Body = ioutil.NopCloser(&bytes.Buffer{})
	}
	defer r.Body.Close()

	if hook, ok := s.impl.(interface {
		ServeHTTP(c *Context, w http.ResponseWriter, r *http.Request)
	}); ok {
		hook.ServeHTTP(args.Context, w, r)
	} else {
		http.NotFound(w, r)
	}

	return nil
}
