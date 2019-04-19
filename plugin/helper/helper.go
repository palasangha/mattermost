package helper

import (
	"github.com/mattermost/mattermost-server/plugin"
)

type helper struct {
	api plugin.API
}

func NewHelper(api plugin.API) *helper {
	return &helper{api: api}
}

func (f *helper) HelloWorld() {
	f.api.LogWarn("Hello world")
}
