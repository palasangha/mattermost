// Copyright (c) 2016-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package app

import (
	"github.com/mattermost/mattermost-server/mlog"
)

// CreateDefaultMemberships adds users to teams and channels based on their group memberships and how those groups are
// configured to sync with teams and channels for group members on or after the given timestamp.
func (a *App) CreateDefaultMemberships(since int64) error {
	teamMembers, appErr := a.TeamMembersToAdd(since)
	if appErr != nil {
		return appErr
	}

	userIDsGroupedByTeamID := make(map[string][]string)

	for _, userTeam := range teamMembers {
		var userIDs []string
		var ok bool
		userIDs, ok = userIDsGroupedByTeamID[userTeam.TeamID]
		if ok {
			userIDs = append(userIDs, userTeam.UserID)
		} else {
			userIDs = []string{}
		}
		userIDsGroupedByTeamID[userTeam.TeamID] = userIDs
	}

	for teamID, userIDs := range userIDsGroupedByTeamID {
		err := a.BulkAddTeamMembers(teamID, userIDs)
		if err != nil {
			return err
		}

		a.Log.Info("added teammembers",
			mlog.Int("num_members_added", len(userIDs)),
			mlog.String("team_id", teamID),
		)
	}

	channelMembers, appErr := a.ChannelMembersToAdd(since)
	if appErr != nil {
		return appErr
	}

	for _, userChannel := range channelMembers {
		channel, err := a.GetChannel(userChannel.ChannelID)
		if err != nil {
			return err
		}

		tmem, err := a.GetTeamMember(channel.TeamId, userChannel.UserID)
		if err != nil && err.Id != "store.sql_team.get_member.missing.app_error" {
			return err
		}

		// First add user to team
		if tmem == nil {
			_, err = a.AddTeamMember(channel.TeamId, userChannel.UserID)
			if err != nil {
				return err
			}
			a.Log.Info("added teammember",
				mlog.String("user_id", userChannel.UserID),
				mlog.String("team_id", channel.TeamId),
			)
		}

		_, err = a.AddChannelMember(userChannel.UserID, channel, "", "")
		if err != nil {
			return err
		}

		a.Log.Info("added channelmember",
			mlog.String("user_id", userChannel.UserID),
			mlog.String("channel_id", userChannel.ChannelID),
		)
	}

	return nil
}

// DeleteGroupConstrainedMemberships deletes team and channel memberships of users who aren't members of the allowed
// groups of all group-constrained teams and channels.
func (a *App) DeleteGroupConstrainedMemberships() error {
	channelMembers, appErr := a.ChannelMembersToRemove()
	if appErr != nil {
		return appErr
	}

	for _, userChannel := range channelMembers {
		channel, err := a.GetChannel(userChannel.ChannelId)
		if err != nil {
			return err
		}

		err = a.RemoveUserFromChannel(userChannel.UserId, "", channel)
		if err != nil {
			return err
		}

		a.Log.Info("removed channelmember",
			mlog.String("user_id", userChannel.UserId),
			mlog.String("channel_id", channel.Id),
		)
	}

	teamMembers, appErr := a.TeamMembersToRemove()
	if appErr != nil {
		return appErr
	}

	for _, userTeam := range teamMembers {
		err := a.RemoveUserFromTeam(userTeam.TeamId, userTeam.UserId, "")
		if err != nil {
			return err
		}

		a.Log.Info("removed teammember",
			mlog.String("user_id", userTeam.UserId),
			mlog.String("team_id", userTeam.TeamId),
		)
	}

	return nil
}
