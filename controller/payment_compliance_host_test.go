package controller_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Uses the real API router and DB-backed dashboard sessions, not a mocked
// privilege context. Its only database is the test's isolated SQLite file.
func TestB21PaymentComplianceCapabilityActualHTTPPermissions(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldCache, oldSecret := common.RedisEnabled, common.MemoryCacheEnabled, common.SessionSecret
	oldRate := common.GlobalApiRateLimitEnable
	oldGinMode := gin.Mode()
	oldPayment := *operation_setting.GetPaymentSetting()
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{"PRIVATE_TEST_OPTION": "must-not-leak"}
	common.OptionMapRWMutex.Unlock()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "b21-compliance.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Option{}, &model.Log{}))
	sqlConnection, err := db.DB()
	require.NoError(t, err)
	sqlConnection.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.SessionSecret = "b21-dashboard-fixture-secret"
	common.GlobalApiRateLimitEnable = false
	*operation_setting.GetPaymentSetting() = operation_setting.PaymentSetting{}
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for gopool.WorkerCount() > 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		require.Zero(t, gopool.WorkerCount(), "finish actual audit work before restoring shared DB")
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetMainDatabaseType(oldType)
		common.SetLogDatabaseType(oldLogType)
		common.RedisEnabled, common.MemoryCacheEnabled, common.SessionSecret = oldRedis, oldCache, oldSecret
		common.GlobalApiRateLimitEnable = oldRate
		gin.SetMode(oldGinMode)
		*operation_setting.GetPaymentSetting() = oldPayment
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, e := db.DB()
		require.NoError(t, e)
		require.NoError(t, sqlDB.Close())
	})
	tokens := map[int]string{}
	for i, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		u := model.User{Id: 216801 + i, Username: []string{"b21-common", "b21-admin", "b21-root"}[i], Password: "unused", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: []string{"b21-common-aff", "b21-admin-aff", "b21-root-aff"}[i]}
		require.NoError(t, db.Create(&u).Error)
		bundle, e := service.CreateLoginSession(u.Id, "password", "127.0.0.1", "b21-real-http")
		require.NoError(t, e)
		tokens[role] = bundle.AccessToken
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.SetApiRouter(engine)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	request := func(t *testing.T, method, path, token string, body []byte) (int, map[string]any) {
		req, e := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		require.NoError(t, e)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, e := server.Client().Do(req)
		require.NoError(t, e)
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		require.NoError(t, e)
		result := map[string]any{}
		if strings.Contains(response.Header.Get("Content-Type"), "application/json") {
			require.NoError(t, common.Unmarshal(raw, &result), "actual status=%d", response.StatusCode)
		} else {
			result["non_json_body"] = string(raw)
		}
		return response.StatusCode, result
	}
	capability := func(t *testing.T, wantConfirmed bool) {
		status, result := request(t, http.MethodGet, "/api/subscription/admin/payment-compliance?terms_version=CLIENT_FAKE_VERSION", tokens[common.RoleAdminUser], nil)
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, true, result["success"])
		assert.Equal(t, map[string]any{"confirmed": wantConfirmed, "terms_version": operation_setting.CurrentComplianceTermsVersion}, result["data"], "only minimal server-authoritative capability, no options/IP/operator audit data")
	}
	t.Run("unconfirmed_admin_can_read", func(t *testing.T) { capability(t, false) })
	t.Run("ordinary_user_forbidden", func(t *testing.T) {
		status, result := request(t, http.MethodGet, "/api/subscription/admin/payment-compliance", tokens[common.RoleCommonUser], nil)
		assert.Equal(t, http.StatusForbidden, status)
		assert.Equal(t, "AUTH_INSUFFICIENT_PRIVILEGE", result["code"])
		assert.NotContains(t, result, "data")
	})
	t.Run("unauthenticated_rejected", func(t *testing.T) {
		status, result := request(t, http.MethodGet, "/api/subscription/admin/payment-compliance", "", nil)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.NotContains(t, result, "data")
	})
	t.Run("admin_cannot_read_root_options_or_confirm", func(t *testing.T) {
		for _, route := range []struct {
			method, path string
			body         []byte
		}{{http.MethodGet, "/api/option/", nil}, {http.MethodPost, "/api/option/payment_compliance", []byte(`{"confirmed":true}`)}} {
			status, result := request(t, route.method, route.path, tokens[common.RoleAdminUser], route.body)
			assert.Equal(t, http.StatusForbidden, status)
			assert.Equal(t, "AUTH_INSUFFICIENT_PRIVILEGE", result["code"])
		}
		assert.False(t, operation_setting.IsPaymentComplianceConfirmed())
		var count int64
		require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
		assert.Zero(t, count)
	})
	t.Run("root_confirms_current_server_terms_admin_can_read", func(t *testing.T) {
		status, result := request(t, http.MethodPost, "/api/option/payment_compliance", tokens[common.RoleRootUser], []byte(`{"confirmed":true,"terms_version":"CLIENT_FAKE_VERSION"}`))
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, result["success"])
		var saved model.Option
		require.NoError(t, db.First(&saved, "`key` = ?", "payment_setting.compliance_terms_version").Error)
		assert.Equal(t, operation_setting.CurrentComplianceTermsVersion, saved.Value)
		capability(t, true)
	})
	t.Run("capability_is_readonly_no_post_confirmation", func(t *testing.T) {
		before := *operation_setting.GetPaymentSetting()
		var countBefore, countAfter int64
		require.NoError(t, db.Model(&model.Option{}).Count(&countBefore).Error)
		status, _ := request(t, http.MethodPost, "/api/subscription/admin/payment-compliance", tokens[common.RoleAdminUser], []byte(`{"confirmed":false,"terms_version":"CLIENT_FAKE_VERSION"}`))
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, before, *operation_setting.GetPaymentSetting())
		require.NoError(t, db.Model(&model.Option{}).Count(&countAfter).Error)
		assert.Equal(t, countBefore, countAfter)
	})
	t.Run("stale_saved_terms_are_not_confirmation", func(t *testing.T) {
		require.NoError(t, model.UpdateOption("payment_setting.compliance_terms_version", "STALE_SERVER_TERMS"))
		assert.True(t, operation_setting.GetPaymentSetting().ComplianceConfirmed)
		capability(t, false)
	})
	t.Run("root_option_permission_control", func(t *testing.T) {
		status, result := request(t, http.MethodGet, "/api/option/", tokens[common.RoleRootUser], nil)
		assert.Equal(t, http.StatusOK, status)
		assert.Equal(t, true, result["success"])
	})
}
