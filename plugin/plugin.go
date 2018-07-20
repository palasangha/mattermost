// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

//go:generate go run interface_generator/main.go

package plugin

import (
	"encoding/gob"
	"net/rpc"

	"github.com/hashicorp/go-plugin"
	"github.com/mattermost/mattermost-server/mlog"
	"github.com/mattermost/mattermost-server/model"
)

// Implements hashicorp/go-plugin/plugin.Plugin interface to connect the hooks of a plugin
type hooksPlugin struct {
	hooks   interface{}
	apiImpl API
	log     *mlog.Logger
}

func (p *hooksPlugin) Server(b *plugin.MuxBroker) (interface{}, error) {
	return &hooksRPCServer{impl: p.hooks, muxBroker: b}, nil
}

func (p *hooksPlugin) Client(b *plugin.MuxBroker, client *rpc.Client) (interface{}, error) {
	return &hooksRPCClient{client: client, log: p.log, muxBroker: b, apiImpl: p.apiImpl}, nil
}

func init() {
	gob.Register([]*model.SlackAttachment{})
	gob.Register([]interface{}{})
	gob.Register(map[string]interface{}{})
}

// These enforce compile time checks to make sure types implement the interface
// If you are getting an error here, you probably need to run `make pluginapi` to
// autogenerate RPC glue code
var _ plugin.Plugin = &hooksPlugin{}
var _ Hooks = &hooksRPCClient{}
