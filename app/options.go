// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package app

import (
	"github.com/mattermost/mattermost-server/store"
)

type Option func(s *Server)

// By default, the app will use the store specified by the configuration. This allows you to
// construct an app with a different store.
//
// The override parameter must be either a store.Store or func(App) store.Store.
func StoreOverride(override interface{}) Option {
	return func(s *Server) {
		switch o := override.(type) {
		case store.Store:
			s.newStore = func() store.Store {
				return o
			}
		case func(*Server) store.Store:
			s.newStore = func() store.Store {
				return o(s)
			}
		default:
			panic("invalid StoreOverride")
		}
	}
}

func ConfigFile(file string) Option {
	return func(s *Server) {
		s.configFile = file
	}
}

func DisableConfigWatch(s *Server) {
	s.disableConfigWatch = true
}
