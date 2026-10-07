package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func optionPersistenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := useFrontendOptionMigrationDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{"review-sentinel": "unchanged"}
	common.OptionMapRWMutex.Unlock()
	secret := common.UnsubscribeSecret
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
		common.UnsubscribeSecret = secret
	})
	return db
}

func TestOptionInitializationReadFailureDoesNotReplaceMemoryOrSecret(t *testing.T) {
	db := optionPersistenceDB(t)
	require.NoError(t, db.Create(&Option{Key: "UnsubscribeSecret", Value: "persisted-existing-secret"}).Error)
	common.UnsubscribeSecret = ""
	forced := errors.New("forced option query failure")
	reads := 0
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:option_init_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			reads++
			if reads == 1 {
				tx.AddError(forced)
			}
		}
	}))
	require.ErrorIs(t, InitOptionMap(), forced)
	assert.Equal(t, map[string]string{"review-sentinel": "unchanged"}, common.OptionMap)
	assert.Empty(t, common.UnsubscribeSecret)
	assert.Equal(t, "persisted-existing-secret", requireOptionValue(t, db, "UnsubscribeSecret"))
}

func TestOptionWriteFailuresDoNotPublish(t *testing.T) {
	for _, phase := range []string{"query", "create", "save", "commit_not_applied", "commit_applied"} {
		t.Run(phase, func(t *testing.T) {
			db := optionPersistenceDB(t)
			if phase != "create" {
				require.NoError(t, db.Create(&Option{Key: "UnsubscribeSecret", Value: "old"}).Error)
			}
			common.UnsubscribeSecret = "old"
			common.OptionMap["UnsubscribeSecret"] = "old"
			forced := errors.New("forced option persistence failure")
			fail := func(tx *gorm.DB) {
				if tx.Statement.Table == "options" {
					tx.AddError(forced)
				}
			}
			switch phase {
			case "query":
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:option_query", fail))
			case "create":
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:option_create", fail))
			case "save":
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:option_save", fail))
			default:
				sqlDB, err := db.DB()
				require.NoError(t, err)
				pool := &batchOutcomeTestPool{DB: sqlDB, mode: phase, forced: forced}
				db.ConnPool, db.Statement.ConnPool = pool, pool
			}
			require.ErrorIs(t, UpdateOption("UnsubscribeSecret", "new"), forced)
			assert.Equal(t, "old", common.UnsubscribeSecret)
			assert.Equal(t, "old", common.OptionMap["UnsubscribeSecret"])
			if phase == "query" {
				require.NoError(t, db.Callback().Query().Remove("test:option_query"))
			}
			if phase == "create" {
				requireOptionMissing(t, db, "UnsubscribeSecret")
			} else {
				expected := "old"
				if phase == "commit_applied" {
					expected = "new"
				}
				assert.Equal(t, expected, requireOptionValue(t, db, "UnsubscribeSecret"))
			}
		})
	}
}

func TestOptionInvalidValuesRejectedBeforePersistenceOrPublication(t *testing.T) {
	cases := []struct {
		key     string
		current func() string
	}{
		{"Chats", setting.Chats2JsonString}, {"AutoGroups", setting.AutoGroups2JsonString},
		{"TopupGroupRatio", common.TopupGroupRatio2JSONString}, {"ModelRequestRateLimitGroup", setting.ModelRequestRateLimitGroup2JSONString},
		{"ModelRatio", ratio_setting.ModelRatio2JSONString}, {"GroupRatio", ratio_setting.GroupRatio2JSONString},
		{"GroupGroupRatio", ratio_setting.GroupGroupRatio2JSONString}, {"UserUsableGroups", setting.UserUsableGroups2JSONString},
		{"CompletionRatio", ratio_setting.CompletionRatio2JSONString}, {"ModelPrice", ratio_setting.ModelPrice2JSONString},
		{"CacheRatio", ratio_setting.CacheRatio2JSONString}, {"CreateCacheRatio", ratio_setting.CreateCacheRatio2JSONString},
		{"ImageRatio", ratio_setting.ImageRatio2JSONString}, {"AudioRatio", ratio_setting.AudioRatio2JSONString},
		{"AudioCompletionRatio", ratio_setting.AudioCompletionRatio2JSONString}, {"PayMethods", operation_setting.PayMethods2JsonString},
		{"AutomaticDisableStatusCodes", operation_setting.AutomaticDisableStatusCodesToString},
		{"AutomaticRetryStatusCodes", operation_setting.AutomaticRetryStatusCodesToString},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			db := optionPersistenceDB(t)
			old := tc.current()
			t.Cleanup(func() { require.NoError(t, updateOptionMap(tc.key, old)) })
			require.NoError(t, db.Create(&Option{Key: tc.key, Value: old}).Error)
			common.OptionMap[tc.key] = old
			require.Error(t, UpdateOption(tc.key, "{invalid"))
			assert.Equal(t, old, requireOptionValue(t, db, tc.key))
			assert.Equal(t, old, common.OptionMap[tc.key])
			assert.Equal(t, old, tc.current())
		})
	}
}

func TestUnsubscribeBootstrapPreservesPersistedWinner(t *testing.T) {
	for _, value := range []string{"existing-winner", "", "missing"} {
		t.Run(value, func(t *testing.T) {
			db := optionPersistenceDB(t)
			if value != "missing" {
				require.NoError(t, db.Create(&Option{Key: "UnsubscribeSecret", Value: value}).Error)
			}
			common.UnsubscribeSecret = ""
			require.NoError(t, ensureUnsubscribeSecret())
			persisted := requireOptionValue(t, db, "UnsubscribeSecret")
			if value == "existing-winner" {
				assert.Equal(t, value, persisted)
			} else {
				assert.Len(t, persisted, 64)
			}
			assert.Equal(t, persisted, common.UnsubscribeSecret)
			assert.Equal(t, persisted, common.OptionMap["UnsubscribeSecret"])
			common.UnsubscribeSecret = ""
			require.NoError(t, ensureUnsubscribeSecret())
			assert.Equal(t, persisted, requireOptionValue(t, db, "UnsubscribeSecret"))
			assert.Equal(t, persisted, common.UnsubscribeSecret)
		})
	}
}

func TestOptionInvalidStoredSnapshotDoesNotPartiallyApply(t *testing.T) {
	for _, loader := range []string{"startup", "sync"} {
		t.Run(loader, func(t *testing.T) {
			db := optionPersistenceDB(t)
			require.NoError(t, db.Create(&[]Option{{Key: "UnsubscribeSecret", Value: "stored"}, {Key: "Chats", Value: "{invalid"}}).Error)
			common.UnsubscribeSecret = "memory"
			oldChats := setting.Chats2JsonString()
			if loader == "startup" {
				require.Error(t, InitOptionMap())
			} else {
				require.Error(t, loadOptionsFromDatabase())
			}
			assert.Equal(t, "memory", common.UnsubscribeSecret)
			assert.Equal(t, oldChats, setting.Chats2JsonString())
			assert.Equal(t, map[string]string{"review-sentinel": "unchanged"}, common.OptionMap)
			assert.Equal(t, "stored", requireOptionValue(t, db, "UnsubscribeSecret"))
		})
	}
}

func TestOptionBulkWriteRollsBackAllValues(t *testing.T) {
	db := optionPersistenceDB(t)
	require.NoError(t, db.Create(&[]Option{{Key: "UnsubscribeSecret", Value: "old"}, {Key: "option-test-other", Value: "old"}}).Error)
	common.UnsubscribeSecret = "old"
	common.OptionMap["UnsubscribeSecret"] = "old"
	common.OptionMap["option-test-other"] = "old"
	forced := errors.New("second write failure")
	writes := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:option_bulk_second", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			writes++
			if writes == 2 {
				tx.AddError(forced)
			}
		}
	}))
	require.ErrorIs(t, UpdateOptionsBulk(map[string]string{"UnsubscribeSecret": "new", "option-test-other": "new"}), forced)
	assert.Equal(t, 2, writes)
	for _, key := range []string{"UnsubscribeSecret", "option-test-other"} {
		assert.Equal(t, "old", requireOptionValue(t, db, key))
		assert.Equal(t, "old", common.OptionMap[key])
	}
	assert.Equal(t, "old", common.UnsubscribeSecret)
}

func TestOptionBootstrapUsesWinnerCreatedAfterStartupRead(t *testing.T) {
	db := optionPersistenceDB(t)
	common.UnsubscribeSecret = "stale-memory"
	read := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:option_other_node_winner", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" && !read {
			read = true
			require.NoError(t, db.Create(&Option{Key: "UnsubscribeSecret", Value: "other-node-winner"}).Error)
		}
	}))
	require.NoError(t, InitOptionMap())
	assert.Equal(t, "other-node-winner", requireOptionValue(t, db, "UnsubscribeSecret"))
	assert.Equal(t, "other-node-winner", common.UnsubscribeSecret)
	assert.Equal(t, "other-node-winner", common.OptionMap["UnsubscribeSecret"])
}

func TestOptionBootstrapFailureDoesNotPublishCandidate(t *testing.T) {
	for _, phase := range []string{"create", "query", "save", "commit_not_applied", "commit_applied"} {
		t.Run(phase, func(t *testing.T) {
			db := optionPersistenceDB(t)
			if phase == "save" {
				require.NoError(t, db.Create(&Option{Key: "UnsubscribeSecret", Value: ""}).Error)
			}
			common.UnsubscribeSecret = "old-memory"
			forced := errors.New("bootstrap persistence failure")
			fail := func(tx *gorm.DB) {
				if tx.Statement.Table == "options" {
					tx.AddError(forced)
				}
			}
			switch phase {
			case "create":
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:bootstrap_create", fail))
			case "query":
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:bootstrap_query", fail))
			case "save":
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:bootstrap_save", fail))
			default:
				sqlDB, err := db.DB()
				require.NoError(t, err)
				pool := &batchOutcomeTestPool{DB: sqlDB, mode: phase, forced: forced}
				db.ConnPool, db.Statement.ConnPool = pool, pool
			}
			require.ErrorIs(t, ensureUnsubscribeSecret(), forced)
			assert.Equal(t, "old-memory", common.UnsubscribeSecret)
			assert.Equal(t, map[string]string{"review-sentinel": "unchanged"}, common.OptionMap)
			if phase == "query" {
				require.NoError(t, db.Callback().Query().Remove("test:bootstrap_query"))
			}
			switch phase {
			case "save":
				assert.Empty(t, requireOptionValue(t, db, "UnsubscribeSecret"))
			case "commit_applied":
				assert.Len(t, requireOptionValue(t, db, "UnsubscribeSecret"), 64)
			default:
				requireOptionMissing(t, db, "UnsubscribeSecret")
			}
		})
	}
}

func TestOptionStartupExistingSecretNeedsNoBootstrapWrite(t *testing.T) {
	db := optionPersistenceDB(t)
	require.NoError(t, db.Create(&Option{Key: "UnsubscribeSecret", Value: "persisted"}).Error)
	common.UnsubscribeSecret = "stale-memory"
	rejectWrite := func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			tx.AddError(errors.New("startup must not write an existing secret"))
		}
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:startup_no_create", rejectWrite))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:startup_no_update", rejectWrite))
	require.NoError(t, InitOptionMap())
	assert.Equal(t, "persisted", common.UnsubscribeSecret)
	assert.Equal(t, "persisted", common.OptionMap["UnsubscribeSecret"])
}

func TestOptionStorageErrorsKeepDriverValuesPrivate(t *testing.T) {
	for _, operation := range []string{"startup", "sync", "single", "bulk", "bootstrap"} {
		t.Run(operation, func(t *testing.T) {
			db := optionPersistenceDB(t)
			forced := errors.New("driver error includes TEST_ONLY_SECRET_VALUE and user:TEST_PASSWORD@database")
			fail := func(tx *gorm.DB) {
				if tx.Statement.Table == "options" {
					tx.AddError(forced)
				}
			}
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:option_private_query", fail))
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:option_private_create", fail))
			var err error
			switch operation {
			case "startup":
				err = InitOptionMap()
			case "sync":
				err = loadOptionsFromDatabase()
			case "single":
				err = UpdateOption("UnsubscribeSecret", "TEST_ONLY_SECRET_VALUE")
			case "bulk":
				err = UpdateOptionsBulk(map[string]string{"UnsubscribeSecret": "TEST_ONLY_SECRET_VALUE"})
			case "bootstrap":
				err = ensureUnsubscribeSecret()
			}
			require.ErrorIs(t, err, forced)
			assert.EqualError(t, err, "failed to access option storage")
			var safe *optionStorageError
			require.ErrorAs(t, err, &safe)
			assert.NotContains(t, err.Error(), "TEST_ONLY_SECRET_VALUE")
			assert.NotContains(t, err.Error(), "TEST_PASSWORD")
		})
	}
}
