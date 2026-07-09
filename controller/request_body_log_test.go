package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserSettingTestDB(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}))
	t.Cleanup(func() { _ = sqlDB.Close() })
}

// TestUpdateUserSettingPreservesRecordRequestBody is the regression guard for the
// admin-control invariant: a monitored user saving their own notification settings
// (self endpoint, UserAuth) must NOT clear the record_request_body flag an admin set
// on them. UpdateUserSetting rebuilds the whole Setting blob from scratch, so any
// admin-only field it omits would be silently wiped, giving the user a self-service
// off switch for surveillance they cannot otherwise disable.
func TestUpdateUserSettingPreservesRecordRequestBody(t *testing.T) {
	setupUserSettingTestDB(t)

	pw, err := common.Password2Hash("x")
	require.NoError(t, err)
	// Seed a user whose admin-set setting has recording enabled.
	user := &model.User{
		Username: "monitored",
		Password: pw,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Setting:  `{"record_request_body":true,"notify_type":"webhook"}`,
	}
	require.NoError(t, model.DB.Create(user).Error)
	require.True(t, user.GetSetting().RecordRequestBody, "precondition: recording enabled")

	// The user saves an unrelated notification preference via the self endpoint.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"notify_type":"email","quota_warning_threshold":0.5,"accept_unset_model_ratio_model":false,"record_ip_log":false}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", user.Id)

	UpdateUserSetting(c)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"success\":true", "settings save should succeed")

	reloaded, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	got := reloaded.GetSetting()
	assert.Equal(t, "email", got.NotifyType, "the notification change must be applied (handler ran)")
	assert.True(t, got.RecordRequestBody,
		"admin-set record_request_body must survive a user's own settings save")
}
