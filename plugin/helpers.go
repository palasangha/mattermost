// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package plugin

import (
	"github.com/mattermost/mattermost-server/model"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

type Helpers interface {
	// EnsureBot ether returns an existing bot user or creates a bot user with
	// the specifications of the passed bot.
	// Returns the id of the bot created or existing.
	EnsureBot(bot *model.Bot) (string, error)

	// Loadi18nBundle loads all localization files in i18n into a bundle and return this
	Loadi18nBundle() (*i18n.Bundle, error)
}

type HelpersImpl struct {
	API API
}
