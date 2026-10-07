package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSafetyDB(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Setup{}, &model.Option{}))
	oldDB, oldType, oldSetup := model.DB, common.MainDatabaseType(), constant.Setup
	oldSelf, oldDemo := operation_setting.SelfUseModeEnabled, operation_setting.DemoSiteEnabled
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{"sentinel": "unchanged"}
	common.OptionMapRWMutex.Unlock()
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	constant.Setup = false
	operation_setting.SelfUseModeEnabled = false
	operation_setting.DemoSiteEnabled = false
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB = oldDB
		common.SetMainDatabaseType(oldType)
		constant.Setup = oldSetup
		operation_setting.SelfUseModeEnabled = oldSelf
		operation_setting.DemoSiteEnabled = oldDemo
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
		gin.SetMode(oldMode)
		require.NoError(t, sqlDB.Close())
	})
	router := gin.New()
	router.GET("/api/setup", GetSetup)
	router.POST("/api/setup", PostSetup)
	return db, router
}

func setupSafetyRequest(t *testing.T, router *gin.Engine, method string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(method, "/api/setup", strings.NewReader(`{"username":"newroot","password":"long-test-password","confirmPassword":"long-test-password","SelfUseModeEnabled":true,"DemoSiteEnabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

func TestSetupFailureLeavesNoPartialInitialization(t *testing.T) {
	for _, phase := range []string{"setup_read", "root_read", "transaction_setup_read", "transaction_root_read", "root_create", "self_mode", "demo_mode", "setup_create"} {
		t.Run(phase, func(t *testing.T) {
			db, router := setupSafetyDB(t)
			forced := errors.New("TEST_PRIVATE_DRIVER_VALUE")
			setupReads, rootReads := 0, 0
			failQuery := func(tx *gorm.DB) {
				if tx.Statement.Table == "setups" {
					setupReads++
				}
				if tx.Statement.Table == "users" {
					rootReads++
				}
				if (phase == "setup_read" && tx.Statement.Table == "setups") || (phase == "root_read" && tx.Statement.Table == "users") ||
					(phase == "transaction_setup_read" && tx.Statement.Table == "setups" && setupReads == 2) || (phase == "transaction_root_read" && tx.Statement.Table == "users" && rootReads == 2) {
					tx.AddError(forced)
				}
			}
			failCreate := func(tx *gorm.DB) {
				if (phase == "root_create" && tx.Statement.Table == "users") || (phase == "setup_create" && tx.Statement.Table == "setups") {
					tx.AddError(forced)
				}
				if option, ok := tx.Statement.Dest.(*model.Option); ok && ((phase == "self_mode" && option.Key == "SelfUseModeEnabled") || (phase == "demo_mode" && option.Key == "DemoSiteEnabled")) {
					tx.AddError(forced)
				}
			}
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:setup_read_fail", failQuery))
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:setup_create_fail", failCreate))
			body := setupSafetyRequest(t, router, http.MethodPost)
			assert.Equal(t, false, body["success"])
			assert.NotContains(t, fmt.Sprint(body["message"]), "TEST_PRIVATE_DRIVER_VALUE")
			require.NoError(t, db.Callback().Query().Remove("test:setup_read_fail"))
			for _, table := range []string{"users", "options", "setups"} {
				var count int64
				require.NoError(t, db.Table(table).Count(&count).Error)
				assert.Zero(t, count, table)
			}
			assert.False(t, constant.Setup)
			assert.False(t, operation_setting.SelfUseModeEnabled)
			assert.False(t, operation_setting.DemoSiteEnabled)
			assert.Equal(t, map[string]string{"sentinel": "unchanged"}, common.OptionMap)
		})
	}
}

func TestSetupRejectsStaleOpenFlagAgainstDatabase(t *testing.T) {
	for _, state := range []string{"setup_noncanonical_id", "existing_root"} {
		t.Run(state, func(t *testing.T) {
			db, router := setupSafetyDB(t)
			if state == "setup_noncanonical_id" {
				require.NoError(t, db.Create(&model.Setup{ID: 17, Version: "existing", InitializedAt: 123}).Error)
			} else {
				require.NoError(t, db.Create(&model.User{Username: "existing", Role: common.RoleRootUser, Password: "existing-hash"}).Error)
			}
			body := setupSafetyRequest(t, router, http.MethodPost)
			assert.Equal(t, false, body["success"])
			var roots, options int64
			require.NoError(t, db.Model(&model.User{}).Where("role = ?", common.RoleRootUser).Count(&roots).Error)
			require.NoError(t, db.Model(&model.Option{}).Count(&options).Error)
			if state == "existing_root" {
				assert.EqualValues(t, 1, roots)
			} else {
				assert.Zero(t, roots)
			}
			assert.Zero(t, options)
			assert.False(t, operation_setting.SelfUseModeEnabled)
			assert.False(t, operation_setting.DemoSiteEnabled)
		})
	}
}

func TestSetupGetReadFailureCannotClaimUninitialized(t *testing.T) {
	db, router := setupSafetyDB(t)
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:setup_get_fail", func(tx *gorm.DB) { tx.AddError(errors.New("TEST_PRIVATE_DRIVER_VALUE")) }))
	body := setupSafetyRequest(t, router, http.MethodGet)
	assert.Equal(t, false, body["success"])
	assert.NotContains(t, fmt.Sprint(body["message"]), "TEST_PRIVATE_DRIVER_VALUE")
	assert.NotContains(t, body, "data")
}

func TestSetupSuccessAndRepeatPreserveWinningAccountAndModes(t *testing.T) {
	db, router := setupSafetyDB(t)
	body := setupSafetyRequest(t, router, http.MethodPost)
	require.Equal(t, true, body["success"])
	assert.True(t, constant.Setup)
	assert.True(t, operation_setting.SelfUseModeEnabled)
	assert.True(t, operation_setting.DemoSiteEnabled)
	var root model.User
	require.NoError(t, db.First(&root).Error)
	assert.Equal(t, "newroot", root.Username)
	assert.Equal(t, common.RoleRootUser, root.Role)
	assert.True(t, common.ValidatePasswordAndHash("long-test-password", root.Password))
	var marker model.Setup
	require.NoError(t, db.First(&marker).Error)
	assert.EqualValues(t, 1, marker.ID)
	constant.Setup = false
	body = setupSafetyRequest(t, router, http.MethodGet)
	require.Equal(t, true, body["success"])
	require.Equal(t, true, body["data"].(map[string]any)["status"])
	body = setupSafetyRequest(t, router, http.MethodPost)
	assert.Equal(t, false, body["success"])
	var roots int64
	require.NoError(t, db.Model(&model.User{}).Count(&roots).Error)
	assert.EqualValues(t, 1, roots)
	var options []model.Option
	require.NoError(t, db.Find(&options).Error)
	require.Len(t, options, 2)
	for _, option := range options {
		assert.Equal(t, "true", option.Value)
	}
}

func TestSetupRechecksRootCreatedAfterPublicPreflight(t *testing.T) {
	db, router := setupSafetyDB(t)
	reads := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:late_root", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			reads++
			if reads == 1 {
				require.NoError(t, db.Create(&model.User{Username: "other-node-root", Role: common.RoleRootUser, Password: "other-node-hash"}).Error)
			}
		}
	}))
	body := setupSafetyRequest(t, router, http.MethodPost)
	assert.Equal(t, false, body["success"])
	var roots []model.User
	require.NoError(t, db.Find(&roots).Error)
	require.Len(t, roots, 1)
	assert.Equal(t, "other-node-root", roots[0].Username)
	var markers, options int64
	require.NoError(t, db.Model(&model.Setup{}).Count(&markers).Error)
	require.NoError(t, db.Model(&model.Option{}).Count(&options).Error)
	assert.Zero(t, markers)
	assert.Zero(t, options)
	assert.False(t, constant.Setup)
	assert.False(t, operation_setting.SelfUseModeEnabled)
	assert.False(t, operation_setting.DemoSiteEnabled)
}
