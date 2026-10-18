package controller_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Casbin and database column names have process-global state without a snapshot
// API. A child test process isolates the real router/session fixture from other
// controller tests without mocking authentication or mutating production state.
func TestChannelOverrideActualHTTPPermissions(t *testing.T) {
	if os.Getenv("LEMONHUB_CHANNEL_OVERRIDE_HTTP_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestChannelOverrideActualHTTPPermissions$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "LEMONHUB_CHANNEL_OVERRIDE_HTTP_CHILD=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		t.Log(string(output))
		return
	}
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldCache, oldSecret := common.RedisEnabled, common.MemoryCacheEnabled, common.SessionSecret
	oldRate, oldMaster, oldMode := common.GlobalApiRateLimitEnable, common.IsMasterNode, gin.Mode()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "b23-readonly.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Option{}, &model.Log{}, &model.Channel{}, &model.Ability{}, &model.CasbinRule{}, &model.AuthzRole{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.SessionSecret = "b23-isolated-session-fixture"
	common.GlobalApiRateLimitEnable, common.IsMasterNode = false, true
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, time.Millisecond, "finish actual audit work before closing fixture DB")
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldType, oldLogType)
		common.RedisEnabled, common.MemoryCacheEnabled, common.SessionSecret = oldRedis, oldCache, oldSecret
		common.GlobalApiRateLimitEnable, common.IsMasterNode = oldRate, oldMaster
		gin.SetMode(oldMode)
		require.NoError(t, sqlDB.Close())
	})
	tokens := map[string]string{}
	ids := map[string]int{}
	for i, identity := range []struct {
		name string
		role int
	}{
		{"ordinary-admin", common.RoleAdminUser}, {"write-only", common.RoleAdminUser},
		{"secret-view", common.RoleAdminUser}, {"root", common.RoleRootUser},
		{"common", common.RoleCommonUser}, {"denied-read", common.RoleAdminUser},
	} {
		u := model.User{Id: 230001 + i, Username: identity.name, Password: "unused", Role: identity.role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: fmt.Sprint("b23-aff-", i)}
		require.NoError(t, db.Create(&u).Error)
		bundle, e := service.CreateLoginSession(u.Id, "password", "127.0.0.1", "b23-fixture")
		require.NoError(t, e)
		tokens[identity.name], ids[identity.name] = bundle.AccessToken, u.Id
	}
	require.NoError(t, authz.SetUserPermissions(ids["write-only"], authz.PermissionsMap{authz.ResourceChannel: {authz.ActionSensitiveWrite: true}}))
	require.NoError(t, authz.SetUserPermissions(ids["secret-view"], authz.PermissionsMap{authz.ResourceChannel: {authz.ActionSecretView: true}}))
	require.NoError(t, authz.SetUserPermissions(ids["denied-read"], authz.PermissionsMap{authz.ResourceChannel: {authz.ActionRead: false}}))
	require.True(t, authz.Can(ids["ordinary-admin"], common.RoleAdminUser, authz.ChannelRead))
	require.True(t, authz.Can(ids["ordinary-admin"], common.RoleAdminUser, authz.ChannelWrite))
	require.False(t, authz.Can(ids["ordinary-admin"], common.RoleAdminUser, authz.ChannelSensitiveWrite))
	require.False(t, authz.Can(ids["ordinary-admin"], common.RoleAdminUser, authz.ChannelSecretView))
	require.True(t, authz.Can(ids["write-only"], common.RoleAdminUser, authz.ChannelSensitiveWrite))
	require.False(t, authz.Can(ids["write-only"], common.RoleAdminUser, authz.ChannelSecretView))
	header := `{"Authorization":"Bearer b23-header-sentinel","X-Vendor-Auth":"b23-arbitrary-header-sentinel"}`
	param := `{"operations":[{"path":"provider_options.credential","mode":"set","value":"b23-param-sentinel"},{"path":"temperature","mode":"set","value":0.7}]}`
	setting := `{"proxy":"http://b23-shadow-secret@proxy.example:8080","Proxy":"http://b23-proxy-user:b23-proxy-password@proxy.example:8080","force_format":false}`
	settings := `{"advanced_custom":null,"ADVANCED_CUSTOM":{"advanced_routes":[{"incoming_path":"/v1/chat/completions","upstream_path":"https://upstream.example/api?token=b23-route-query","auth":{"type":"header","name":"X-Provider-Key","value":"b23-route-sentinel"}}]},"disable_store":true}`
	baseURL := "https://b23-base-user:b23-base-password@upstream.example?api_key=b23-base-query"
	tag := "b23-tag"
	channel := model.Channel{Id: 230001, Type: 1, Name: "b23-channel", Status: common.ChannelStatusEnabled, Key: "b23-key-sentinel", Models: "gpt-4o-mini", Group: "default", Tag: &tag, HeaderOverride: &header, ParamOverride: &param, Setting: &setting, OtherSettings: settings, BaseURL: &baseURL}
	require.NoError(t, channel.Insert())
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.SetApiRouter(engine)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	request := func(t *testing.T, method, path, identity string, body []byte) (int, map[string]any, []byte) {
		t.Helper()
		req, e := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		require.NoError(t, e)
		if token := tokens[identity]; token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, e := server.Client().Do(req)
		require.NoError(t, e)
		defer resp.Body.Close()
		raw, e := io.ReadAll(resp.Body)
		require.NoError(t, e)
		result := map[string]any{}
		require.NoError(t, common.Unmarshal(raw, &result))
		return resp.StatusCode, result, raw
	}
	routes := []struct{ name, path string }{
		{"list", "/api/channel/"}, {"list-tag", "/api/channel/?tag_mode=true"},
		{"search", "/api/channel/search?keyword=b23-channel"}, {"search-tag", "/api/channel/search?keyword=b23-channel&tag_mode=true"},
		{"detail", "/api/channel/230001"},
	}
	for _, identity := range []string{"ordinary-admin", "write-only", "secret-view", "root"} {
		for _, route := range routes {
			t.Run(identity+"/"+route.name, func(t *testing.T) {
				status, body, raw := request(t, http.MethodGet, route.path, identity, nil)
				require.Equal(t, http.StatusOK, status)
				require.Equal(t, true, body["success"])
				var item map[string]any
				if route.name == "detail" {
					item = body["data"].(map[string]any)
				} else {
					data := body["data"].(map[string]any)
					items := data["items"].([]any)
					require.Len(t, items, 1)
					item = items[0].(map[string]any)
				}
				assert.Equal(t, float64(channel.Id), item["id"])
				assert.Equal(t, "gpt-4o-mini", item["models"])
				assert.Equal(t, true, item["param_override_configured"])
				assert.Equal(t, true, item["header_override_configured"])
				assert.NotContains(t, string(raw), "b23-key-sentinel")
				if identity == "root" || identity == "secret-view" {
					assert.Equal(t, header, item["header_override"])
					assert.Equal(t, param, item["param_override"])
					assert.Equal(t, setting, item["setting"])
					assert.Equal(t, settings, item["settings"])
					assert.Equal(t, baseURL, item["base_url"])
				} else {
					assert.Nil(t, item["param_override"])
					assert.Nil(t, item["header_override"])
					assert.NotContains(t, string(raw), "b23-header-sentinel")
					assert.NotContains(t, string(raw), "b23-arbitrary-header-sentinel")
					assert.NotContains(t, string(raw), "b23-param-sentinel")
					assert.Equal(t, true, item["proxy_configured"])
					assert.Equal(t, true, item["advanced_custom_configured"])
					assert.Equal(t, true, item["base_url_redacted"])
					assert.Nil(t, item["base_url"])
					for _, secret := range []string{"b23-shadow-secret", "b23-proxy-user", "b23-proxy-password", "b23-route-query", "b23-route-sentinel", "b23-base-user", "b23-base-password", "b23-base-query"} {
						assert.NotContains(t, string(raw), secret)
					}
				}
			})
		}
	}
	t.Run("auth-controls", func(t *testing.T) {
		for _, identity := range []string{"common", "denied-read", ""} {
			status, body, raw := request(t, http.MethodGet, "/api/channel/230001", identity, nil)
			want := http.StatusForbidden
			if identity == "" {
				want = http.StatusUnauthorized
			}
			assert.Equal(t, want, status)
			assert.Equal(t, false, body["success"])
			assert.NotContains(t, string(raw), "b23-header-sentinel")
			assert.NotContains(t, string(raw), "b23-param-sentinel")
		}
	})
	t.Run("routing-update-preserves-and-must-not-echo", func(t *testing.T) {
		status, body, raw := request(t, http.MethodPut, "/api/channel/", "ordinary-admin", []byte(`{"id":230001,"name":"routing-edit","models":"gpt-4o-mini","group":"default"}`))
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		var saved model.Channel
		require.NoError(t, db.First(&saved, channel.Id).Error)
		assert.Equal(t, "routing-edit", saved.Name)
		assert.Equal(t, &header, saved.HeaderOverride)
		assert.Equal(t, &param, saved.ParamOverride)
		assert.NotContains(t, string(raw), "b23-header-sentinel")
		assert.NotContains(t, string(raw), "b23-param-sentinel")
	})
	t.Run("ambiguous-hidden-fields-refuse-implicit-overwrite", func(t *testing.T) {
		for _, fixture := range []struct {
			name, column, requestField, original, replacement string
		}{
			{
				name: "proxy mixed aliases and ignored null", column: "setting", requestField: "setting",
				original:    `{"proxy":"http://b23-retained-secret@proxy.example:8080","Proxy":null,"force_format":false}`,
				replacement: `{"force_format":true}`,
			},
			{
				name: "proxy identical duplicate keys", column: "setting", requestField: "setting",
				original:    `{"proxy":"http://b23-retained-secret@proxy.example:8080","proxy":null,"force_format":false}`,
				replacement: `{"force_format":true}`,
			},
			{
				name: "advanced custom mixed aliases merge objects", column: "settings", requestField: "settings",
				original:    `{"advanced_custom":{"advanced_routes":[{"incoming_path":"/v1/chat/completions","upstream_path":"/v1/chat/completions","auth":{"type":"header","name":"X-Provider-Key","value":"b23-retained-secret"}}]},"ADVANCED_CUSTOM":{},"disable_store":true}`,
				replacement: `{"disable_store":false}`,
			},
			{
				name: "advanced custom identical duplicate keys", column: "settings", requestField: "settings",
				original:    `{"advanced_custom":{"advanced_routes":[{"incoming_path":"/v1/chat/completions","upstream_path":"/v1/chat/completions","auth":{"type":"header","name":"X-Provider-Key","value":"b23-retained-secret"}}]},"advanced_custom":{},"disable_store":true}`,
				replacement: `{"disable_store":false}`,
			},
		} {
			t.Run(fixture.name, func(t *testing.T) {
				require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update(fixture.column, fixture.original).Error)
				before, err := model.GetChannelById(channel.Id, true)
				require.NoError(t, err)
				_, detail, detailRaw := request(t, http.MethodGet, "/api/channel/230001", "write-only", nil)
				require.Equal(t, true, detail["success"])
				configuredKey := "advanced_custom_configured"
				if fixture.column == "setting" {
					configuredKey = "proxy_configured"
				}
				assert.Equal(t, true, detail["data"].(map[string]any)[configuredKey], "duplicate fields must not be advertised as empty")
				assert.NotContains(t, string(detailRaw), "b23-retained-secret")
				if fixture.column == "setting" {
					assert.Equal(t, "http://b23-retained-secret@proxy.example:8080", before.GetSetting().Proxy, "null does not erase an already decoded string")
				} else {
					advanced := before.GetOtherSettings().AdvancedCustom
					require.NotNil(t, advanced)
					require.Len(t, advanced.Routes, 1, "an empty repeated object preserves previously decoded fields")
					assert.Equal(t, "b23-retained-secret", advanced.Routes[0].Auth.Value)
				}
				payload, err := common.Marshal(map[string]any{"id": channel.Id, "name": "must-not-persist", fixture.requestField: fixture.replacement})
				require.NoError(t, err)
				status, body, raw := request(t, http.MethodPut, "/api/channel/", "write-only", payload)
				require.Equal(t, http.StatusOK, status)
				assert.Equal(t, false, body["success"])
				assert.Contains(t, body["message"], "repeated sensitive fields")
				assert.NotContains(t, string(raw), "b23-retained-secret")
				after, err := model.GetChannelById(channel.Id, true)
				require.NoError(t, err)
				assert.Equal(t, before, after, "a rejected implicit edit must preserve the full stored channel")
			})
		}
	})
	t.Run("write-only-json-edit-preserves-omitted-hidden-secrets", func(t *testing.T) {
		// Explicit replacement can resolve old duplicate members. Keep one
		// case-insensitive alias and an unknown nested value to exercise lossless
		// preservation during the following ordinary edit.
		singleSetting := `{"Proxy":"http://b23-proxy-user:b23-proxy-password@proxy.example:8080","force_format":false}`
		singleSettings := `{"ADVANCED_CUSTOM":{"advanced_routes":[{"incoming_path":"/v1/chat/completions","upstream_path":"https://upstream.example/api?token=b23-route-query","auth":{"type":"header","name":"X-Provider-Key","value":"b23-route-sentinel"}}],"unknown_nested":"b23-provider-private-extension"},"disable_store":true}`
		payload, err := common.Marshal(map[string]any{"id": channel.Id, "setting": singleSetting, "settings": singleSettings})
		require.NoError(t, err)
		status, body, raw := request(t, http.MethodPut, "/api/channel/", "write-only", payload)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		assert.NotContains(t, string(raw), "b23-provider-private-extension")
		status, body, raw = request(t, http.MethodPut, "/api/channel/", "write-only", []byte(`{"id":230001,"setting":"{\"force_format\":true}","settings":"{\"disable_store\":false}"}`))
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		saved, err := model.GetChannelById(channel.Id, true)
		require.NoError(t, err)
		assert.True(t, saved.GetSetting().ForceFormat)
		assert.Equal(t, "http://b23-proxy-user:b23-proxy-password@proxy.example:8080", saved.GetSetting().Proxy)
		require.NotNil(t, saved.GetOtherSettings().AdvancedCustom)
		assert.Equal(t, "b23-route-sentinel", saved.GetOtherSettings().AdvancedCustom.Routes[0].Auth.Value)
		assert.False(t, saved.GetOtherSettings().DisableStore)
		assert.Contains(t, saved.OtherSettings, `"unknown_nested":"b23-provider-private-extension"`)
		assert.Equal(t, &baseURL, saved.BaseURL)
		for _, secret := range []string{"b23-proxy-password", "b23-route-sentinel", "b23-base-query"} {
			assert.NotContains(t, string(raw), secret)
		}
		status, body, _ = request(t, http.MethodPut, "/api/channel/", "write-only", []byte(`{"id":230001,"setting":"{\"proxy\":\"\"}","settings":"{\"advanced_custom\":null}","base_url":""}`))
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		saved, err = model.GetChannelById(channel.Id, true)
		require.NoError(t, err)
		assert.Empty(t, saved.GetSetting().Proxy)
		assert.Nil(t, saved.GetOtherSettings().AdvancedCustom)
		require.NotNil(t, saved.BaseURL)
		assert.Empty(t, *saved.BaseURL)
	})
	t.Run("write-permission-is-separate-from-secret-read", func(t *testing.T) {
		for _, identity := range []string{"ordinary-admin", "secret-view"} {
			for _, replacement := range [][]byte{
				[]byte(`{"id":230001,"header_override":"{\"X-New-Auth\":\"b23-new-header\"}"}`),
				[]byte(`{"id":230001,"header_override":"","param_override":""}`),
				[]byte(`{"id":230001,"header_override":null,"param_override":null}`),
			} {
				status, body, raw := request(t, http.MethodPut, "/api/channel/", identity, replacement)
				require.Equal(t, http.StatusOK, status)
				assert.Equal(t, false, body["success"])
				assert.NotContains(t, string(raw), "b23-header-sentinel")
				var saved model.Channel
				require.NoError(t, db.First(&saved, channel.Id).Error)
				assert.Equal(t, &header, saved.HeaderOverride)
				assert.Equal(t, &param, saved.ParamOverride)
			}
		}
	})
	t.Run("write-only-explicit-replacement-and-clear-preserve-other-rule", func(t *testing.T) {
		replacement := `{"X-New-Auth":"b23-new-header"}`
		bodyBytes, err := common.Marshal(map[string]any{"id": channel.Id, "header_override": replacement})
		require.NoError(t, err)
		status, body, raw := request(t, http.MethodPut, "/api/channel/", "write-only", bodyBytes)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		assert.NotContains(t, string(raw), "b23-new-header")
		assert.NotContains(t, string(raw), "b23-param-sentinel")
		data := body["data"].(map[string]any)
		assert.Nil(t, data["header_override"])
		assert.Equal(t, true, data["header_override_configured"])
		saved, err := model.GetChannelById(channel.Id, true)
		require.NoError(t, err)
		assert.Equal(t, &replacement, saved.HeaderOverride)
		assert.Equal(t, &param, saved.ParamOverride, "parameter routing configuration remains available to internal relay readers")
		assert.Equal(t, "b23-key-sentinel", saved.Key)

		status, body, raw = request(t, http.MethodPut, "/api/channel/", "write-only", []byte(`{"id":230001,"header_override":""}`))
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		data = body["data"].(map[string]any)
		assert.Equal(t, false, data["header_override_configured"])
		assert.Equal(t, true, data["param_override_configured"])
		assert.NotContains(t, string(raw), "b23-param-sentinel")
		saved, err = model.GetChannelById(channel.Id, true)
		require.NoError(t, err)
		require.NotNil(t, saved.HeaderOverride)
		assert.Empty(t, *saved.HeaderOverride)
		assert.Equal(t, &param, saved.ParamOverride)

		bodyBytes, err = common.Marshal(map[string]any{"id": channel.Id, "param_override": `{"temperature":0.5}`})
		require.NoError(t, err)
		status, body, _ = request(t, http.MethodPut, "/api/channel/", "root", bodyBytes)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		data = body["data"].(map[string]any)
		assert.Equal(t, `{"temperature":0.5}`, data["param_override"])
		assert.Equal(t, "", data["header_override"])
	})
	t.Run("absent-and-whitespace-overrides-have-no-configured-marker", func(t *testing.T) {
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{"param_override": nil, "header_override": "  \n"}).Error)
		status, body, _ := request(t, http.MethodGet, "/api/channel/230001", "ordinary-admin", nil)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		assert.Equal(t, false, data["param_override_configured"])
		assert.Equal(t, false, data["header_override_configured"])
		assert.Nil(t, data["param_override"])
		assert.Nil(t, data["header_override"])
	})
	t.Run("complete-key-still-requires-root-and-security-proof", func(t *testing.T) {
		for _, identity := range []string{"ordinary-admin", "write-only", "secret-view", "root"} {
			status, body, raw := request(t, http.MethodPost, "/api/channel/230001/key", identity, []byte(`{}`))
			require.Equal(t, http.StatusForbidden, status)
			assert.Equal(t, false, body["success"])
			assert.NotContains(t, string(raw), "b23-key-sentinel")
			if identity == "root" {
				assert.Equal(t, "SECURITY_PROOF_REQUIRED", body["code"])
			}
		}
	})

}
