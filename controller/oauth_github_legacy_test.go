package controller

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupGitHubIdentityControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRegisterEnabled := common.RegisterEnabled
	var dialector gorm.Dialector = sqlite.Open(":memory:")
	databaseType := common.DatabaseTypeSQLite
	if rawDSN := strings.TrimSpace(os.Getenv("LEMONHUB_AUDIT_GITHUB_DSN")); rawDSN != "" {
		config, err := mysqldriver.ParseDSN(rawDSN)
		require.NoError(t, err)
		require.Equal(t, "tcp", config.Net, "GitHub identity tests require an isolated local TCP database")
		host, port, err := net.SplitHostPort(config.Addr)
		require.NoError(t, err)
		require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, host)
		require.Contains(t, []string{"33957", "33980"}, port)
		require.Equal(t, "lemonhub_github_test_batch2", config.DBName, "refusing to alter any other database")
		config.ParseTime = true
		dialector = mysql.Open(config.FormatDSN())
		databaseType = common.DatabaseTypeMySQL
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.SetMainDatabaseType(databaseType)
	common.RegisterEnabled = false
	require.NoError(t, i18n.Init())
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RegisterEnabled = previousRegisterEnabled
		assert.NoError(t, sqlDB.Close())
	})
	if databaseType == common.DatabaseTypeMySQL {
		var selectedDatabase string
		require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
		require.Equal(t, "lemonhub_github_test_batch2", selectedDatabase)
	}
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ExternalIdentityClaim{}, &model.AuthFlow{}))
	for _, table := range []string{"auth_flows", "external_identity_claims", "users"} {
		require.NoError(t, db.Exec("DELETE FROM "+table).Error)
	}
	return db
}

func TestGitHubLegacyUsernameCannotAuthenticateOrRewriteBinding(t *testing.T) {
	for _, login := range []string{"reused-github-login", "123456"} {
		t.Run(login, func(t *testing.T) {
			db := setupGitHubIdentityControllerTest(t)
			common.RegisterEnabled = login != "123456"
			legacyUser := model.User{
				Username: "legacy-owner", Password: "password", AffCode: "legacy-owner",
				Email: "unverified@example.test", GitHubId: login, SiteId: 7,
			}
			require.NoError(t, db.Create(&legacyUser).Error)
			require.NoError(t, model.UpdateUserBindColumn(legacyUser.Id, "github_id", login))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/github", nil)
			common.SetContextKey(ctx, constant.ContextKeySiteId, 7)

			user, err := findOrCreateOAuthUser(ctx, &oauth.GitHubProvider{}, &oauth.OAuthUser{
				ProviderUserID: "987654", Username: login, Email: legacyUser.Email,
				Extra: map[string]any{"legacy_id": login},
			}, "")

			if login == "123456" {
				require.ErrorAs(t, err, new(*OAuthRegistrationDisabledError), "numeric login names must not match another account's numeric subject")
			} else {
				require.ErrorAs(t, err, new(*OAuthLegacyBindingNotConfirmedError), "a current login name and unverified public email are not account ownership evidence")
			}
			assert.Nil(t, user)
			var unchanged model.User
			require.NoError(t, db.First(&unchanged, legacyUser.Id).Error)
			assert.Equal(t, login, unchanged.GitHubId)
			var claim model.ExternalIdentityClaim
			require.NoError(t, db.Where("provider = ? AND user_id = ?", "github", legacyUser.Id).First(&claim).Error)
			assert.Equal(t, login, claim.Subject)
			assert.Equal(t, 7, claim.SiteId)
			var users int64
			require.NoError(t, db.Model(&model.User{}).Count(&users).Error)
			assert.EqualValues(t, 1, users)
		})
	}
}

func TestGitHubNumericBindingLoginRemainsSiteScoped(t *testing.T) {
	db := setupGitHubIdentityControllerTest(t)
	owners := []model.User{
		{Username: "first-owner", Password: "password", AffCode: "first-owner", GitHubId: "987654", SiteId: 7},
		{Username: "second-owner", Password: "password", AffCode: "second-owner", GitHubId: "987654", SiteId: 8},
	}
	for index := range owners {
		require.NoError(t, db.Create(&owners[index]).Error)
		require.NoError(t, model.UpdateUserBindColumn(owners[index].Id, "github_id", "987654"))
	}
	for _, owner := range owners {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/github", nil)
		common.SetContextKey(ctx, constant.ContextKeySiteId, owner.SiteId)
		user, err := findOrCreateOAuthUser(ctx, &oauth.GitHubProvider{}, &oauth.OAuthUser{
			ProviderUserID: "987654", Extra: map[string]any{"legacy_id": "changed-login"},
		}, "")
		require.NoError(t, err)
		require.NotNil(t, user)
		assert.Equal(t, owner.Id, user.Id)
		assert.Equal(t, "987654", user.GitHubId)
	}
}

type githubIdentityTestProvider struct {
	oauth.GitHubProvider
	identity oauth.OAuthUser
}

func (*githubIdentityTestProvider) IsEnabled() bool { return true }
func (*githubIdentityTestProvider) ExchangeToken(context.Context, string, *gin.Context) (*oauth.OAuthToken, error) {
	return &oauth.OAuthToken{}, nil
}
func (provider *githubIdentityTestProvider) GetUserInfo(context.Context, *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	return &provider.identity, nil
}

func TestGitHubAuthenticatedRelinkIgnoresReusedUsername(t *testing.T) {
	db := setupGitHubIdentityControllerTest(t)
	owners := []model.User{
		{Username: "relink-owner", Password: "password", AffCode: "relink-owner", GitHubId: "old-login", SiteId: 7},
		{Username: "username-owner", Password: "password", AffCode: "username-owner", GitHubId: "reused-login", SiteId: 7},
		{Username: "other-site-owner", Password: "password", AffCode: "other-site-owner", GitHubId: "987654", SiteId: 8},
	}
	for index := range owners {
		require.NoError(t, db.Create(&owners[index]).Error)
		require.NoError(t, model.UpdateUserBindColumn(owners[index].Id, "github_id", owners[index].GitHubId))
	}
	provider := &githubIdentityTestProvider{identity: oauth.OAuthUser{
		ProviderUserID: "987654", Extra: map[string]any{"legacy_id": "reused-login"},
	}}
	oauth.Register("github-identity-test", provider)
	t.Cleanup(func() { oauth.Unregister("github-identity-test") })
	flowToken, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeOAuth, Provider: "github-identity-test", Intent: model.AuthFlowIntentBind,
		UserId: owners[0].Id, SessionId: "authenticated-owner-session", Payload: `{}`,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", owners[0].Id)
		c.Set("session_id", "authenticated-owner-session")
		c.Set("auth_version", int64(1))
		c.Set("session_version", int64(1))
		// Bind ownership must follow the authenticated user's site, not this Host scope.
		common.SetContextKey(c, constant.ContextKeySiteId, 8)
		c.Next()
	})
	router.GET("/api/oauth/:provider", HandleOAuth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/oauth/github-identity-test?state="+flowToken+"&code=test", nil))
	var result struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.True(t, result.Success, response.Body.String())
	var owner model.User
	require.NoError(t, db.First(&owner, owners[0].Id).Error)
	assert.Equal(t, "987654", owner.GitHubId)
	var claims []model.ExternalIdentityClaim
	require.NoError(t, db.Where("provider = ? AND subject = ?", "github", "987654").Order("site_id").Find(&claims).Error)
	require.Len(t, claims, 2)
	assert.Equal(t, owners[0].Id, claims[0].UserId)
	assert.Equal(t, owners[2].Id, claims[1].UserId)
	var legacyOwner model.User
	require.NoError(t, db.First(&legacyOwner, owners[1].Id).Error)
	assert.Equal(t, "reused-login", legacyOwner.GitHubId)
}

func TestGitHubLegacyCallbackDeclinesWithPrivateRelinkGuidance(t *testing.T) {
	db := setupGitHubIdentityControllerTest(t)
	const legacyID = "reused-login-private-marker"
	legacyUser := model.User{
		Username: "legacy-owner-private-marker", Password: "password", AffCode: "legacy-owner",
		Email: "owner-private-marker@example.test", GitHubId: legacyID, SiteId: 7,
	}
	require.NoError(t, db.Create(&legacyUser).Error)
	provider := &githubIdentityTestProvider{identity: oauth.OAuthUser{
		ProviderUserID: "987654321", Extra: map[string]any{"legacy_id": legacyID},
	}}
	oauth.Register("github-identity-test", provider)
	t.Cleanup(func() { oauth.Unregister("github-identity-test") })
	for _, language := range []string{i18n.LangEn, i18n.LangZhCN, i18n.LangZhTW} {
		t.Run(language, func(t *testing.T) {
			flowToken, cookies := startOAuthLoginFlow(t, "github-identity-test")
			require.Len(t, cookies, 1)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeySiteId, 7)
				common.SetContextKey(c, constant.ContextKeyLanguage, language)
				c.Next()
			})
			router.GET("/api/oauth/:provider", HandleOAuth)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/oauth/github-identity-test?state="+flowToken+"&code=test", nil)
			request.AddCookie(cookies[0])
			router.ServeHTTP(response, request)
			var result struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success)
			assert.Equal(t, i18n.Translate(language, i18n.MsgOAuthNotAutoLinked, providerParams("GitHub")), result.Message)
			assert.NotEqual(t, i18n.MsgOAuthNotAutoLinked, result.Message)
			for _, privateValue := range []string{legacyID, provider.identity.ProviderUserID, legacyUser.Username, legacyUser.Email} {
				assert.NotContains(t, response.Body.String(), privateValue)
			}
			cleared := response.Result().Cookies()
			require.Len(t, cleared, 1)
			assert.Equal(t, cookies[0].Name, cleared[0].Name)
			assert.Equal(t, -1, cleared[0].MaxAge)
			assert.NotContains(t, response.Body.String(), "access_token")
			_, err := model.GetAuthFlow(flowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth})
			assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
		})
	}
}
