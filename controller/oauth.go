package controller

import (
	"crypto/hmac"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const oauthAuthFlowTTL = 10 * time.Minute

type oauthStateRequest struct {
	Provider string `json:"provider"`
	Intent   string `json:"intent"`
	Aff      string `json:"aff,omitempty"`
}

type oauthFlowPayload struct {
	AffiliateCode    string `json:"affiliate_code,omitempty"`
	LoginBrowserHash string `json:"login_browser_hash,omitempty"`
}

// Each login attempt has its own cookie so concurrent tabs cannot overwrite
// each other's binding. Lax permits the provider's top-level return navigation.
func oauthLoginBrowserCookie(state, secret string, maxAge int) *http.Cookie {
	name := "new_api_oauth_" + state
	path := "/api/oauth"
	if common.SessionCookieSecure {
		// The __Host- prefix prevents an untrusted sibling subdomain from
		// injecting a Domain cookie for an attacker's known state and secret.
		name = "__Host-" + name
		path = "/"
	}
	return &http.Cookie{
		Name:     name,
		Value:    secret,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   common.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func oauthLoginBrowserHash(secret string) string {
	return common.GenerateHMACWithKey([]byte("oauth-login-browser-v1:"+common.SessionSecret), secret)
}

func validateOAuthLoginBrowser(c *gin.Context, state string, flow *model.AuthFlow) (oauthFlowPayload, bool) {
	var payload oauthFlowPayload
	browserCookies := c.Request.CookiesNamed(oauthLoginBrowserCookie(state, "", 0).Name)
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil ||
		len(browserCookies) != 1 || browserCookies[0].Value == "" || payload.LoginBrowserHash == "" ||
		!hmac.Equal([]byte(payload.LoginBrowserHash), []byte(oauthLoginBrowserHash(browserCookies[0].Value))) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
		return oauthFlowPayload{}, false
	}
	return payload, true
}

// Non-standard providers carry state separately from their signed credentials.
// In particular, Telegram's signature must still cover the unchanged query.
func requireBrowserBoundProviderLogin(c *gin.Context, provider string) (string, bool) {
	state := c.GetHeader("X-OAuth-State")
	flow, err := model.GetAuthFlow(state, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeOAuth, Provider: provider, Intent: model.AuthFlowIntentLogin,
	})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
		return "", false
	}
	_, ok := validateOAuthLoginBrowser(c, state, flow)
	return state, ok
}

// providerParams returns map with Provider key for i18n templates
func providerParams(name string) map[string]any {
	return map[string]any{"Provider": name}
}

// GenerateOAuthCode generates a state code for OAuth CSRF protection
func GenerateOAuthCode(c *gin.Context) {
	var request oauthStateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	request.Provider = strings.TrimSpace(request.Provider)
	request.Intent = strings.TrimSpace(request.Intent)
	request.Aff = strings.TrimSpace(request.Aff)
	nonstandardLogin := request.Intent == model.AuthFlowIntentLogin && (request.Provider == "wechat" || request.Provider == "telegram")
	if (oauth.GetProvider(request.Provider) == nil && !nonstandardLogin) ||
		(request.Intent != model.AuthFlowIntentLogin && request.Intent != model.AuthFlowIntentBind) ||
		len(request.Aff) > 32 ||
		(request.Intent == model.AuthFlowIntentBind && request.Aff != "") {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userID := 0
	sessionID := ""
	if request.Intent == model.AuthFlowIntentBind {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "绑定操作需要登录"})
			return
		}
		userID = identity.UserID
		sessionID = identity.SessionID
	}
	flowPayload := oauthFlowPayload{AffiliateCode: request.Aff}
	browserSecret := ""
	if request.Intent == model.AuthFlowIntentLogin {
		var err error
		browserSecret, err = common.GenerateRandomCharsKey(64)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		flowPayload.LoginBrowserHash = oauthLoginBrowserHash(browserSecret)
	}
	payload, err := common.Marshal(flowPayload)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiresAt := time.Now().Add(oauthAuthFlowTTL)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeOAuth,
		Provider:  request.Provider,
		Intent:    request.Intent,
		UserId:    userID,
		SessionId: sessionID,
		Payload:   string(payload),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Intent == model.AuthFlowIntentLogin {
		http.SetCookie(c.Writer, oauthLoginBrowserCookie(state, browserSecret, int(oauthAuthFlowTTL.Seconds())))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"flow_token": state,
			"expires_at": expiresAt.Unix(),
		},
	})
}

// HandleOAuth handles OAuth callback for all standard OAuth providers
func HandleOAuth(c *gin.Context) {
	providerName := c.Param("provider")
	provider := oauth.GetProvider(providerName)
	if provider == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": i18n.T(c, i18n.MsgOAuthUnknownProvider),
		})
		return
	}

	// 1. Validate state (CSRF protection)
	state := c.Query("state")
	pendingFlow, err := model.GetAuthFlow(state, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
	})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": i18n.T(c, i18n.MsgOAuthStateInvalid),
		})
		return
	}

	consumeMatch := model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeOAuth,
		Provider: providerName,
		Intent:   pendingFlow.Intent,
	}
	var payload oauthFlowPayload
	// 2. Bind flows are bound to the live dashboard Session that created them.
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		identity, ok := middleware.GetSessionAuthIdentity(c)
		if !ok || identity.UserID != pendingFlow.UserId || identity.SessionID != pendingFlow.SessionId {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgOAuthStateInvalid),
			})
			return
		}
		consumeMatch.UserId = identity.UserID
		consumeMatch.SessionId = identity.SessionID
	} else if pendingFlow.Intent == model.AuthFlowIntentLogin {
		// A state is not sufficient on its own: an attacker knows the state and
		// authorization code for their own account and can send both to a victim.
		// Require the independent secret delivered only to the initiating browser.
		var ok bool
		payload, ok = validateOAuthLoginBrowser(c, state, pendingFlow)
		if !ok {
			return
		}
	} else {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// 3. Check if provider is enabled
	if !provider.IsEnabled() {
		common.ApiErrorI18n(c, i18n.MsgOAuthNotEnabled, providerParams(provider.GetName()))
		return
	}

	// 4. Handle error from provider
	errorCode := c.Query("error")
	if errorCode != "" {
		if _, err := model.ConsumeAuthFlow(state, consumeMatch); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
			return
		}
		if pendingFlow.Intent == model.AuthFlowIntentLogin {
			http.SetCookie(c.Writer, oauthLoginBrowserCookie(state, "", -1))
		}
		errorDescription := c.Query("error_description")
		if errorDescription == "" {
			errorDescription = errorCode
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": errorDescription,
		})
		return
	}
	if pendingFlow.Intent == model.AuthFlowIntentBind {
		handleOAuthBind(c, provider, pendingFlow, state)
		return
	}

	// 5. Exchange code for token
	code := c.Query("code")
	token, err := provider.ExchangeToken(c.Request.Context(), code, c)
	if err != nil {
		handleOAuthError(c, err)
		return
	}

	// 6. Get user info
	oauthUser, err := provider.GetUserInfo(c.Request.Context(), token)
	if err != nil {
		handleOAuthError(c, err)
		return
	}
	_, err = model.ConsumeAuthFlow(state, consumeMatch)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
		return
	}
	http.SetCookie(c.Writer, oauthLoginBrowserCookie(state, "", -1))

	// 7. Find or create user
	user, err := findOrCreateOAuthUser(c, provider, oauthUser, payload.AffiliateCode)
	if err != nil {
		switch err.(type) {
		case *OAuthUserDeletedError:
			common.ApiErrorI18n(c, i18n.MsgOAuthUserDeleted)
		case *OAuthRegistrationDisabledError:
			common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
		case *OAuthLegacyBindingNotConfirmedError:
			common.ApiErrorI18n(c, i18n.MsgOAuthNotAutoLinked, providerParams(provider.GetName()))
		default:
			common.ApiError(c, err)
		}
		return
	}

	// 8. Check user status
	if user.Status != common.UserStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgOAuthUserBanned)
		return
	}

	// 9. Setup login
	setupLogin(user, c)
}

// handleOAuthBind handles binding OAuth account to existing user
func handleOAuthBind(c *gin.Context, provider oauth.Provider, pendingFlow *model.AuthFlow, flowToken string) {
	// Exchange code for token
	code := c.Query("code")
	token, err := provider.ExchangeToken(c.Request.Context(), code, c)
	if err != nil {
		handleOAuthError(c, err)
		return
	}

	// Get user info
	oauthUser, err := provider.GetUserInfo(c.Request.Context(), token)
	if err != nil {
		handleOAuthError(c, err)
		return
	}
	oauthUser.ProviderUserID, err = model.NormalizeExternalIdentitySubject(oauthUser.ProviderUserID)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// Derive tenant scope from the authenticated owner rather than Host. Dashboard
	// sessions can reach the callback through another configured domain; the claim
	// transaction uses the owner's authoritative site and this precheck must match it.
	userId := pendingFlow.UserId
	siteId, err := model.GetUserSiteId(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Only the provider's permanent subject proves ownership. A reused GitHub
	// login name must not prevent an authenticated user from relinking their account.
	if provider.IsUserIDTaken(oauthUser.ProviderUserID, siteId) {
		common.ApiErrorI18n(c, i18n.MsgOAuthAlreadyBound, providerParams(provider.GetName()))
		return
	}
	if _, err := model.ConsumeAuthFlow(flowToken, model.AuthFlowMatch{
		Purpose:   model.AuthFlowPurposeOAuth,
		Provider:  pendingFlow.Provider,
		Intent:    model.AuthFlowIntentBind,
		UserId:    pendingFlow.UserId,
		SessionId: pendingFlow.SessionId,
	}); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": i18n.T(c, i18n.MsgOAuthStateInvalid)})
		return
	}

	// Handle binding based on provider type
	if genericProvider, ok := provider.(*oauth.GenericOAuthProvider); ok {
		// Custom provider: use user_oauth_bindings table
		err = model.UpdateUserOAuthBinding(userId, genericProvider.GetProviderId(), oauthUser.ProviderUserID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		// Built-in provider: 只更新绑定列。完整快照的 user.Update 会把读取时刻的
		// role/status/group 一并写回，覆盖并发发生的封禁、降权或分组变更。
		err = model.UpdateUserBindColumn(userId, provider.ProviderUserIDColumn(), oauthUser.ProviderUserID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	common.ApiSuccessI18n(c, i18n.MsgOAuthBindSuccess, gin.H{
		"action": "bind",
	})
}

// findOrCreateOAuthUser finds existing user or creates new user
func findOrCreateOAuthUser(c *gin.Context, provider oauth.Provider, oauthUser *oauth.OAuthUser, affiliateCode string) (*model.User, error) {
	user := &model.User{}
	var err error
	oauthUser.ProviderUserID, err = model.NormalizeExternalIdentitySubject(oauthUser.ProviderUserID)
	if err != nil {
		return nil, err
	}

	// Resolve the sub-site from the request Host (0 = main site). All identity lookups
	// and the new-user insert below are scoped to it so sub-sites stay isolated.
	siteId := middleware.GetRequestSiteId(c)

	// Check if user already exists with new ID
	if provider.IsUserIDTaken(oauthUser.ProviderUserID, siteId) {
		err := provider.FillUserByProviderID(user, oauthUser.ProviderUserID, siteId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, &OAuthUserDeletedError{}
			}
			return nil, err
		}
		// Check if user has been deleted
		if user.Id == 0 {
			return nil, &OAuthUserDeletedError{}
		}
		return user, nil
	}

	// GitHub login names can be reassigned. A legacy match is never ownership
	// evidence, even when the public profile email also matches. Require the user
	// to sign in another way and relink through the authenticated bind flow.
	// All-digit login names are excluded: they share the existing numeric-ID
	// namespace, so comparing them as legacy names would block unrelated accounts.
	legacyID, _ := oauthUser.Extra["legacy_id"].(string)
	if provider.ProviderUserIDColumn() == "github_id" &&
		strings.ContainsFunc(legacyID, func(r rune) bool { return r < '0' || r > '9' }) &&
		provider.IsUserIDTaken(legacyID, siteId) {
		common.SysLog("[OAuth] legacy GitHub binding login declined: account verification required")
		return nil, &OAuthLegacyBindingNotConfirmedError{}
	}

	// User doesn't exist, create new user if registration is enabled
	if !common.RegisterEnabled {
		return nil, &OAuthRegistrationDisabledError{}
	}

	// Set up new user
	user.Username = provider.GetProviderPrefix() + strconv.Itoa(model.GetMaxUserId()+1)

	if oauthUser.Username != "" {
		if exists, err := model.CheckUserExistOrDeleted(oauthUser.Username, "", siteId); err == nil && !exists {
			// 防止索引退化
			if len(oauthUser.Username) <= model.UserNameMaxLength {
				user.Username = oauthUser.Username
			}
		}
	}

	if oauthUser.DisplayName != "" {
		user.DisplayName = oauthUser.DisplayName
	} else if oauthUser.Username != "" {
		user.DisplayName = oauthUser.Username
	} else {
		user.DisplayName = provider.GetName() + " User"
	}
	if oauthUser.Email != "" {
		user.Email = oauthUser.Email
	}
	user.Role = common.RoleCommonUser
	user.Status = common.UserStatusEnabled
	// Bind the new account to the resolved sub-site so it is isolated from other sites.
	user.SiteId = siteId

	// Handle affiliate code
	inviterId := 0
	if affiliateCode != "" {
		inviterId, _ = model.GetUserIdByAffCode(affiliateCode)
	}

	// Use transaction to ensure user creation and OAuth binding are atomic
	if genericProvider, ok := provider.(*oauth.GenericOAuthProvider); ok {
		// Custom provider: create user and binding in a transaction
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			// Create user
			if err := user.InsertWithTx(tx, inviterId); err != nil {
				return err
			}

			// Create OAuth binding
			binding := &model.UserOAuthBinding{
				UserId:         user.Id,
				ProviderId:     genericProvider.GetProviderId(),
				ProviderUserId: oauthUser.ProviderUserID,
			}
			if err := model.CreateUserOAuthBindingWithTx(tx, binding); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return nil, err
		}

		// Perform post-transaction tasks (logs, sidebar config, inviter rewards)
		user.FinalizeOAuthUserCreation(inviterId)
	} else {
		// Built-in provider: persist the user column and its site-scoped durable
		// identity claim in the same insertion transaction.
		provider.SetProviderUserID(user, oauthUser.ProviderUserID)
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			return user.InsertWithTx(tx, inviterId)
		})
		if err != nil {
			return nil, err
		}

		// Perform post-transaction tasks
		user.FinalizeOAuthUserCreation(inviterId)
	}

	return user, nil
}

// Error types for OAuth
type OAuthUserDeletedError struct{}

func (e *OAuthUserDeletedError) Error() string {
	return "user has been deleted"
}

type OAuthRegistrationDisabledError struct{}

func (e *OAuthRegistrationDisabledError) Error() string {
	return "registration is disabled"
}

type OAuthLegacyBindingNotConfirmedError struct{}

func (e *OAuthLegacyBindingNotConfirmedError) Error() string {
	return "legacy GitHub binding requires account verification"
}

// handleOAuthError handles OAuth errors and returns translated message
func handleOAuthError(c *gin.Context, err error) {
	switch e := err.(type) {
	case *oauth.OAuthError:
		if e.Params != nil {
			common.ApiErrorI18n(c, e.MsgKey, e.Params)
		} else {
			common.ApiErrorI18n(c, e.MsgKey)
		}
	case *oauth.AccessDeniedError:
		common.ApiErrorMsg(c, e.Message)
	case *oauth.TrustLevelError:
		common.ApiErrorI18n(c, i18n.MsgOAuthTrustLevelLow)
	default:
		common.ApiError(c, err)
	}
}
