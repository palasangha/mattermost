// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/plugin"
	"github.com/mattermost/mattermost-server/store"
	"github.com/mattermost/mattermost-server/utils"
)

func (a *App) CheckForClienSideCert(r *http.Request) (string, string, string) {
	pem := r.Header.Get("X-SSL-Client-Cert")                // mapped to $ssl_client_cert from nginx
	subject := r.Header.Get("X-SSL-Client-Cert-Subject-DN") // mapped to $ssl_client_s_dn from nginx
	email := ""

	if len(subject) > 0 {
		for _, v := range strings.Split(subject, "/") {
			kv := strings.Split(v, "=")
			if len(kv) == 2 && kv[0] == "emailAddress" {
				email = kv[1]
			}
		}
	}

	return pem, subject, email
}

func (a *App) AuthenticateUserForLogin(id, loginId, password, mfaToken string, ldapOnly bool) (user *model.User, err *model.AppError) {
	// Do statistics
	defer func() {
		if a.Metrics != nil {
			if user == nil || err != nil {
				a.Metrics.IncrementLoginFail()
			} else {
				a.Metrics.IncrementLogin()
			}
		}
	}()

	if len(password) == 0 {
		err := model.NewAppError("AuthenticateUserForLogin", "api.user.login.blank_pwd.app_error", nil, "", http.StatusBadRequest)
		return nil, err
	}

	// Get the MM user we are trying to login
	if user, err = a.GetUserForLogin(id, loginId); err != nil {
		return nil, err
	}

	// If client side cert is enable and it's checking as a primary source
	// then trust the proxy and cert that the correct user is supplied and allow
	// them access
	if *a.Config().ExperimentalSettings.ClientSideCertEnable && *a.Config().ExperimentalSettings.ClientSideCertCheck == model.CLIENT_SIDE_CERT_CHECK_PRIMARY_AUTH {
		return user, nil
	}

	// and then authenticate them
	if user, err = a.authenticateUser(user, password, mfaToken); err != nil {
		return nil, err
	}

	if a.PluginsReady() {
		var rejectionReason string
		pluginContext := &plugin.Context{}
		a.Plugins.RunMultiPluginHook(func(hooks plugin.Hooks) bool {
			rejectionReason = hooks.UserWillLogIn(pluginContext, user)
			return rejectionReason == ""
		}, plugin.UserWillLogInId)

		if rejectionReason != "" {
			return nil, model.NewAppError("AuthenticateUserForLogin", "Login rejected by plugin: "+rejectionReason, nil, "", http.StatusBadRequest)
		}

		a.Go(func() {
			pluginContext := &plugin.Context{}
			a.Plugins.RunMultiPluginHook(func(hooks plugin.Hooks) bool {
				hooks.UserHasLoggedIn(pluginContext, user)
				return true
			}, plugin.UserHasLoggedInId)
		})
	}

	return user, nil
}

func (a *App) GetUserForLogin(id, loginId string) (*model.User, *model.AppError) {
	enableUsername := *a.Config().EmailSettings.EnableSignInWithUsername
	enableEmail := *a.Config().EmailSettings.EnableSignInWithEmail

	// If we are given a userID then fail if we can't find a user with that ID
	if len(id) != 0 {
		if user, err := a.GetUser(id); err != nil {
			if err.Id != store.MISSING_ACCOUNT_ERROR {
				err.StatusCode = http.StatusInternalServerError
				return nil, err
			} else {
				err.StatusCode = http.StatusBadRequest
				return nil, err
			}
		} else {
			return user, nil
		}
	}

	// Try to get the user by username/email
	if result := <-a.Srv.Store.User().GetForLogin(loginId, enableUsername, enableEmail); result.Err == nil {
		return result.Data.(*model.User), nil
	}

	// Try to get the user with LDAP if enabled
	if *a.Config().LdapSettings.Enable && a.Ldap != nil {
		if ldapUser, err := a.Ldap.GetUser(loginId); err == nil {
			if user, err := a.GetUserByAuth(ldapUser.AuthData, model.USER_AUTH_SERVICE_LDAP); err == nil {
				return user, nil
			}
			return ldapUser, nil
		}
	}

	return nil, model.NewAppError("GetUserForLogin", "store.sql_user.get_for_login.app_error", nil, "", http.StatusBadRequest)
}

func (a *App) DoLogin(w http.ResponseWriter, r *http.Request, user *model.User, deviceId string) (*model.Session, *model.AppError) {
	sessionCreationTime := time.Now()
	sessionCreationTimeMilliseconds := utils.MillisFromTime(sessionCreationTime)
	session := &model.Session{
		UserId:   user.Id,
		Roles:    user.GetRawRoles(),
		DeviceId: deviceId,
		IsOAuth:  false,
		CreateAt: sessionCreationTimeMilliseconds,
	}
	session.GenerateCSRF()
	if r.Header.Get(model.HEADER_USE_REFRESH) == "true" {
		session.AddProp(model.SESSION_PROP_IS_REFRESHABLE_KEY, model.SESSION_PROP_IS_REFRESHABLE_VALUE)
		session.AddProp(model.SESSION_PROP_LAST_REFRESHED_KEY, strconv.Itoa(int(sessionCreationTimeMilliseconds)))
	}
	session.SetUserAgent(r.UserAgent())

	// A special case where we logout of all other sessions with the same Id
	isMobile := len(deviceId) > 0
	if isMobile {
		if err := a.RevokeSessionsForDeviceId(user.Id, deviceId, ""); err != nil {
			err.StatusCode = http.StatusInternalServerError
			return nil, err
		}
	}

	var err *model.AppError
	if session, err = a.CreateSession(session); err != nil {
		err.StatusCode = http.StatusInternalServerError
		return nil, err
	}

	a.SetHTTPAuthInformation(w, r, session)

	return session, nil
}

func (a *App) DoRefresh(w http.ResponseWriter, r *http.Request) (*model.Session, *model.AppError) {
	sessionSettings := &a.Config().SessionSettings

	token, tokenLocation := ParseAuthTokenFromRequest(r)
	// CSRF Check
	if tokenLocation == TokenLocationCookie {
		if r.Header.Get(model.HEADER_REQUESTED_WITH) != model.HEADER_REQUESTED_WITH_XML {
			return nil, model.NewAppError("ServeHTTP", "api.context.session_expired.app_error", nil, "token="+token+" Appears to be a CSRF attempt", http.StatusUnauthorized)
		}
	}

	session, err := a.GetSession(token)
	if session == nil || session.IsExpired(sessionSettings) || !session.IsRefreshable() {
		return nil, model.NewAppError("ServeHTTP", "api.context.session_expired.app_error", nil, "token="+token, http.StatusUnauthorized)
	}

	oldSessionId := session.Id
	a.Go(func() {
		<-time.After(10 * time.Second)
		// Revoke existing session
		a.RevokeSessionById(oldSessionId)
	})

	// Prepare new session.
	sessionRefreshTime := time.Now()
	sessionRefreshTimeMilliseconds := utils.MillisFromTime(sessionRefreshTime)
	session.AddProp(model.SESSION_PROP_LAST_REFRESHED_KEY, strconv.FormatInt(sessionRefreshTimeMilliseconds, 10))
	session.GenerateCSRF()
	session.SetUserAgent(r.UserAgent())
	session.Token = ""
	session.Id = ""

	if session, err = a.CreateSession(session); err != nil {
		err.StatusCode = http.StatusInternalServerError
		return nil, err
	}
	a.SetHTTPAuthInformation(w, r, session)

	return session, nil
}

func (a *App) SetHTTPAuthInformation(w http.ResponseWriter, r *http.Request, session *model.Session) {
	w.Header().Set(model.HEADER_TOKEN, session.Token)

	secure := true
	if GetProtocol(r) == "http" {
		secure = false
	}

	domain := a.GetCookieDomain()
	expiresAt := session.ExpiresAt(&a.Config().SessionSettings)
	maxAge := int(time.Until(expiresAt) / time.Second)
	sessionCookie := &http.Cookie{
		Name:     model.SESSION_COOKIE_TOKEN,
		Value:    session.Token,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  expiresAt,
		HttpOnly: true,
		Domain:   domain,
		Secure:   secure,
	}

	userCookie := &http.Cookie{
		Name:    model.SESSION_COOKIE_USER,
		Value:   session.UserId,
		Path:    "/",
		MaxAge:  maxAge,
		Expires: expiresAt,
		Domain:  domain,
		Secure:  secure,
	}

	http.SetCookie(w, sessionCookie)
	http.SetCookie(w, userCookie)
}

func GetProtocol(r *http.Request) string {
	if r.Header.Get(model.HEADER_FORWARDED_PROTO) == "https" || r.TLS != nil {
		return "https"
	} else {
		return "http"
	}
}
