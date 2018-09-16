// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package model

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	SESSION_COOKIE_TOKEN              = "MMAUTHTOKEN"
	SESSION_COOKIE_USER               = "MMUSERID"
	SESSION_CACHE_SIZE                = 35000
	SESSION_PROP_PLATFORM             = "platform"
	SESSION_PROP_OS                   = "os"
	SESSION_PROP_BROWSER              = "browser"
	SESSION_PROP_TYPE                 = "type"
	SESSION_PROP_USER_ACCESS_TOKEN_ID = "user_access_token_id"
	SESSION_PROP_IS_REFRESHABLE_KEY   = "is_refreshable"
	SESSION_PROP_IS_REFRESHABLE_VALUE = "true"
	SESSION_PROP_LAST_REFRESHED_KEY   = "last_refreshed"
	SESSION_TYPE_USER_ACCESS_TOKEN    = "UserAccessToken"
	SESSION_ACTIVITY_TIMEOUT          = 1000 * 60 * 5 // 5 minutes
)

type Session struct {
	Id             string        `json:"id"`
	Token          string        `json:"token"`
	CreateAt       int64         `json:"create_at"`
	LastActivityAt int64         `json:"last_activity_at"`
	UserId         string        `json:"user_id"`
	DeviceId       string        `json:"device_id"`
	Roles          string        `json:"roles"`
	IsOAuth        bool          `json:"is_oauth"`
	Props          StringMap     `json:"props"`
	TeamMembers    []*TeamMember `json:"team_members" db:"-"`
}

func (me *Session) DeepCopy() *Session {
	copySession := *me

	if me.Props != nil {
		copySession.Props = CopyStringMap(me.Props)
	}

	if me.TeamMembers != nil {
		copySession.TeamMembers = make([]*TeamMember, len(me.TeamMembers))
		for index, tm := range me.TeamMembers {
			copySession.TeamMembers[index] = new(TeamMember)
			*copySession.TeamMembers[index] = *tm
		}
	}

	return &copySession
}

func (me *Session) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}

func SessionFromJson(data io.Reader) *Session {
	var me *Session
	json.NewDecoder(data).Decode(&me)
	return me
}

func (me *Session) PreSave() {
	if me.Id == "" {
		me.Id = NewId()
	}

	if me.Token == "" {
		me.Token = NewId()
	}

	if me.CreateAt == 0 {
		me.CreateAt = GetMillis()
	}
	me.LastActivityAt = GetMillis()

	if me.Props == nil {
		me.Props = make(map[string]string)
	}
}

func (me *Session) Sanitize() {
	me.Token = ""
}

// IsExpire Returns true if the absolute timeout of the session has expired.
// The session my still be refreshable.
func (me *Session) IsExpired(settings *SessionSettings) bool {
	return time.Now().After(me.ExpiresAt(settings))
}

// IsRefreshable returns true if the client has indicated the session is refreshable
func (me *Session) IsRefreshable() bool {
	return me.Props[SESSION_PROP_IS_REFRESHABLE_KEY] == SESSION_PROP_IS_REFRESHABLE_VALUE
}

// IsUserAccessToken returns true if the session is associated with a user access token.
func (me *Session) IsUserAccessToken() bool {
	return me.Props[SESSION_PROP_TYPE] == SESSION_TYPE_USER_ACCESS_TOKEN
}

// NeedsRefresh Returns true if the session needs to be refreshed.
// Should only be called for web and mobile sessions.
// Should call IsExpired first to determine if the session is absolutely expired.
func (me *Session) NeedsRefresh(settings *SessionSettings) bool {
	return time.Now().After(me.RefreshAt(settings))
}

// RefreshAt returns the time at which the session will need to be refreshed
// according to the current session length settings.
func (me *Session) RefreshAt(settings *SessionSettings) time.Time {
	validLength := time.Duration(0)
	if me.IsOAuth {
		return time.Unix(0, 0)
	} else if me.IsMobileApp() {
		validLength = time.Duration(*settings.MobileRenewalTimeoutMinutes) * time.Minute
	} else if me.IsUserAccessToken() {
		// User access tokens never need refreshing
		validLength = time.Hour * 24 * 365 * 100
	} else {
		validLength = time.Duration(*settings.WebRenewalTimeoutMinutes) * time.Minute
	}
	refreshed := me.LastRefreshTime()
	return refreshed.Add(validLength)
}

// ExpiresAt returns the time at which the session will absolutely expire
// according to the current session length settings.
func (me *Session) ExpiresAt(settings *SessionSettings) time.Time {
	validLength := time.Duration(0)
	if me.IsOAuth {
		validLength = time.Duration(*settings.OAuthTimeoutMinutes) * time.Minute
	} else if me.IsMobileApp() {
		validLength = time.Duration(*settings.MobileTimeoutMinutes) * time.Minute
	} else if me.IsUserAccessToken() {
		// User access token sessions don't expire
		validLength = time.Hour * 24 * 365 * 100
	} else {
		validLength = time.Duration(*settings.WebTimeoutMinutes) * time.Minute
	}
	created := time.Unix(0, me.CreateAt*int64(time.Millisecond))
	return created.Add(validLength)
}

// ExpiresAtMilliseconds is the same as ExpiresAt except returns the
// unix time in milliseconds
func (me *Session) ExpiresAtMilliseconds(settings *SessionSettings) int64 {
	return me.ExpiresAt(settings).UnixNano() / int64(time.Millisecond)
}

// ShoudCache returns true if the session should be put into the cache after a read.
func (me *Session) ShouldCache(settings *SessionSettings) bool {
	return !me.NeedsRefresh(settings) && !me.IsExpired(settings)
}

// SetUserAgent given a user agent string will parse and set props on the session
// relating it to the given user agent string.
func (me *Session) SetUserAgent(uaString string) {
	/*ua := uasurfer.Parse(uaString)

	plat := getPlatformName(ua)
	os := getOSName(ua)
	bname := getBrowserName(ua, uaString)
	bversion := getBrowserVersion(ua, uaString)

	session.AddProp(SESSION_PROP_PLATFORM, plat)
	session.AddProp(SESSION_PROP_OS, os)
	session.AddProp(SESSION_PROP_BROWSER, fmt.Sprintf("%v/%v", bname, bversion))*/
}

func (me *Session) AddProp(key string, value string) {

	if me.Props == nil {
		me.Props = make(map[string]string)
	}

	me.Props[key] = value
}

func (me *Session) LastRefreshTime() time.Time {
	if me.Props == nil {
		return time.Unix(0, 0)
	}

	lastRefreshMilliseconds, err := strconv.Atoi(me.Props[SESSION_PROP_LAST_REFRESHED_KEY])
	if err != nil {
		return time.Unix(0, 0)
	}

	return time.Unix(0, int64(lastRefreshMilliseconds)*int64(time.Millisecond))
}

func (me *Session) GetTeamByTeamId(teamId string) *TeamMember {
	for _, team := range me.TeamMembers {
		if team.TeamId == teamId {
			return team
		}
	}

	return nil
}

func (me *Session) IsMobileApp() bool {
	return len(me.DeviceId) > 0
}

func (me *Session) GetUserRoles() []string {
	return strings.Fields(me.Roles)
}

func (me *Session) GenerateCSRF() string {
	token := NewId()
	me.AddProp("csrf", token)
	return token
}

func (me *Session) GetCSRF() string {
	if me.Props == nil {
		return ""
	}

	return me.Props["csrf"]
}

func SessionsToJson(o []*Session) string {
	if b, err := json.Marshal(o); err != nil {
		return "[]"
	} else {
		return string(b)
	}
}

func SessionsFromJson(data io.Reader) []*Session {
	var o []*Session
	json.NewDecoder(data).Decode(&o)
	return o
}
