package model

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelSafetyDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldType, oldSetup := DB, common.MainDatabaseType(), constant.Setup
	oldSelf, oldDemo := operation_setting.SelfUseModeEnabled, operation_setting.DemoSiteEnabled
	oldMap := common.OptionMap
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "setup.sqlite")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Setup{}, &Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	constant.Setup = false
	operation_setting.SelfUseModeEnabled = false
	operation_setting.DemoSiteEnabled = false
	common.OptionMap = map[string]string{"sentinel": "unchanged"}
	t.Cleanup(func() {
		DB = oldDB
		common.SetMainDatabaseType(oldType)
		constant.Setup = oldSetup
		operation_setting.SelfUseModeEnabled = oldSelf
		operation_setting.DemoSiteEnabled = oldDemo
		common.OptionMap = oldMap
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func TestSetupReadsDistinguishMissingFromFailure(t *testing.T) {
	db := setupModelSafetyDB(t)
	found, err := GetSetup()
	require.NoError(t, err)
	assert.Nil(t, found)
	root, err := RootUserExists()
	require.NoError(t, err)
	assert.False(t, root)
	forced := errors.New("PRIVATE_TEST_DRIVER_DETAILS")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:setup_model_read", func(tx *gorm.DB) { tx.AddError(forced) }))
	found, err = GetSetup()
	require.ErrorIs(t, err, forced)
	assert.Nil(t, found)
	assert.NotContains(t, err.Error(), "PRIVATE_TEST")
	root, err = RootUserExists()
	require.ErrorIs(t, err, forced)
	assert.False(t, root)
	assert.NotContains(t, err.Error(), "PRIVATE_TEST")
	for _, old := range []bool{false, true} {
		constant.Setup = old
		require.ErrorIs(t, CheckSetup(), forced)
		assert.Equal(t, old, constant.Setup)
	}
}

func TestCheckSetupLegacyMigrationAndRootScope(t *testing.T) {
	for _, state := range []string{"empty", "root", "disabled_root", "subsite_root", "soft_deleted_root", "arbitrary_marker"} {
		t.Run(state, func(t *testing.T) {
			db := setupModelSafetyDB(t)
			expected := state != "empty" && state != "soft_deleted_root"
			if state == "arbitrary_marker" {
				require.NoError(t, db.Create(&Setup{ID: 19, Version: "legacy", InitializedAt: 123}).Error)
			} else if state != "empty" {
				root := User{Username: "legacy-root", Password: "preserved-hash", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Quota: 98765}
				if state == "disabled_root" {
					root.Status = common.UserStatusDisabled
				}
				if state == "subsite_root" {
					root.SiteId = 77
				}
				require.NoError(t, db.Create(&root).Error)
				if state == "soft_deleted_root" {
					require.NoError(t, db.Delete(&root).Error)
				}
				exists, err := RootUserExists()
				require.NoError(t, err)
				assert.Equal(t, state != "soft_deleted_root", exists)
			}
			require.NoError(t, CheckSetup())
			assert.Equal(t, expected, constant.Setup)
			require.NoError(t, CheckSetup())
			var markers []Setup
			require.NoError(t, db.Find(&markers).Error)
			if expected {
				require.Len(t, markers, 1)
				if state == "arbitrary_marker" {
					assert.EqualValues(t, 19, markers[0].ID)
					assert.Equal(t, "legacy", markers[0].Version)
					assert.EqualValues(t, 123, markers[0].InitializedAt)
				} else {
					assert.EqualValues(t, 1, markers[0].ID)
				}
			} else {
				assert.Empty(t, markers)
			}
			var options int64
			require.NoError(t, db.Model(&Option{}).Count(&options).Error)
			assert.Zero(t, options)
			if state != "empty" && state != "arbitrary_marker" {
				var root User
				require.NoError(t, db.Unscoped().First(&root).Error)
				assert.Equal(t, "preserved-hash", root.Password)
				assert.Equal(t, 98765, root.Quota)
			}
		})
	}
}

func TestCheckSetupLegacyMarkerFailureDoesNotPublish(t *testing.T) {
	db := setupModelSafetyDB(t)
	require.NoError(t, db.Create(&User{Username: "root", Password: "hash", Role: common.RoleRootUser}).Error)
	forced := errors.New("marker failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:legacy_marker_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "setups" {
			tx.AddError(forced)
		}
	}))
	require.ErrorIs(t, CheckSetup(), forced)
	assert.False(t, constant.Setup)
	var count int64
	require.NoError(t, db.Model(&Setup{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSetupCommitOutcomeDoesNotPublishOrReplay(t *testing.T) {
	for _, operation := range []string{"initialize", "legacy_marker"} {
		for _, mode := range []string{"commit_not_applied", "commit_applied"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				db := setupModelSafetyDB(t)
				if operation == "legacy_marker" {
					require.NoError(t, db.Create(&User{Username: "root", Password: "hash", Role: common.RoleRootUser}).Error)
				}
				sqlDB, err := db.DB()
				require.NoError(t, err)
				forced := errors.New("PRIVATE_COMMIT_FAILURE")
				pool := &batchOutcomeTestPool{DB: sqlDB, mode: mode, forced: forced}
				db.ConnPool, db.Statement.ConnPool = pool, pool
				if operation == "initialize" {
					err = InitializeSetup("newroot", "prehashed-test", true, true)
				} else {
					err = CheckSetup()
				}
				require.ErrorIs(t, err, forced)
				assert.NotContains(t, err.Error(), "PRIVATE_COMMIT")
				assert.Equal(t, 1, pool.begins, "no automatic replay after uncertain commit")
				assert.False(t, constant.Setup)
				assert.False(t, operation_setting.SelfUseModeEnabled)
				assert.False(t, operation_setting.DemoSiteEnabled)
				assert.Equal(t, map[string]string{"sentinel": "unchanged"}, common.OptionMap)
				var markers, roots, options int64
				require.NoError(t, db.Model(&Setup{}).Count(&markers).Error)
				require.NoError(t, db.Model(&User{}).Count(&roots).Error)
				require.NoError(t, db.Model(&Option{}).Count(&options).Error)
				if mode == "commit_applied" {
					assert.EqualValues(t, 1, markers)
					assert.EqualValues(t, 1, roots)
					if operation == "initialize" {
						assert.EqualValues(t, 2, options)
					} else {
						assert.Zero(t, options)
					}
				} else {
					assert.Zero(t, markers)
					assert.Zero(t, options)
					if operation == "initialize" {
						assert.Zero(t, roots)
					} else {
						assert.EqualValues(t, 1, roots)
					}
				}
			})
		}
	}
}

func TestSetupConcurrentEmptyDatabaseHasOneWinner(t *testing.T) {
	db := setupModelSafetyDB(t)
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var queries atomic.Int32
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:setup_race_barrier", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && queries.Add(1) <= 2 {
			ready <- struct{}{}
			<-release
		}
	}))
	results := make(chan error, 2)
	go func() { results <- InitializeSetup("root-one", "hash-one", true, false) }()
	go func() { results <- InitializeSetup("root-two", "hash-two", false, true) }()
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("both independent transactions must see the empty root set")
		}
	}
	close(release)
	successes := 0
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err == nil {
				successes++
			}
		case <-time.After(10 * time.Second):
			t.Fatal("setup transaction did not finish")
		}
	}
	require.Equal(t, 1, successes)
	var roots []User
	require.NoError(t, db.Find(&roots).Error)
	require.Len(t, roots, 1)
	var markers []Setup
	require.NoError(t, db.Find(&markers).Error)
	require.Len(t, markers, 1)
	assert.EqualValues(t, 1, markers[0].ID)
	assert.Equal(t, roots[0].Username == "root-one", operation_setting.SelfUseModeEnabled)
	assert.Equal(t, roots[0].Username == "root-two", operation_setting.DemoSiteEnabled)
	assert.Equal(t, common.OptionMap["SelfUseModeEnabled"], requireOptionValue(t, db, "SelfUseModeEnabled"))
	assert.Equal(t, common.OptionMap["DemoSiteEnabled"], requireOptionValue(t, db, "DemoSiteEnabled"))
	require.ErrorIs(t, InitializeSetup("third-root", "hash-three", true, true), ErrSetupAlreadyInitialized)
}

func TestSetupFailurePreservesPreexistingModeOptions(t *testing.T) {
	db := setupModelSafetyDB(t)
	require.NoError(t, db.Create(&[]Option{{Key: "SelfUseModeEnabled", Value: "false"}, {Key: "DemoSiteEnabled", Value: "false"}}).Error)
	forced := errors.New("second mode write failed")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:setup_existing_mode_failure", func(tx *gorm.DB) {
		if value, ok := tx.Statement.Dest.(*Option); ok && value.Key == "DemoSiteEnabled" {
			tx.AddError(forced)
		}
	}))
	require.ErrorIs(t, InitializeSetup("newroot", "test-hash", true, true), forced)
	assert.Equal(t, "false", requireOptionValue(t, db, "SelfUseModeEnabled"))
	assert.Equal(t, "false", requireOptionValue(t, db, "DemoSiteEnabled"))
	var roots, markers int64
	require.NoError(t, db.Model(&User{}).Count(&roots).Error)
	require.NoError(t, db.Model(&Setup{}).Count(&markers).Error)
	assert.Zero(t, roots)
	assert.Zero(t, markers)
	assert.False(t, constant.Setup)
	assert.Equal(t, map[string]string{"sentinel": "unchanged"}, common.OptionMap)
}
