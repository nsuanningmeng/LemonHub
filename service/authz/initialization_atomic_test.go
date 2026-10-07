package authz

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func authzAtomicFixture(t *testing.T, db *gorm.DB) ([]model.CasbinRule, []model.AuthzRole) {
	t.Helper()
	require.NoError(t, Init(db))
	preserved := []model.CasbinRule{
		newRule("p", []string{UserSubject(42), ResourceChannel, ActionRead, EffectDeny}),
		newRule("p", []string{RoleSubject("custom"), ResourceChannel, ActionWrite, EffectAllow}),
	}
	require.NoError(t, db.Create(&preserved).Error)
	require.NoError(t, db.Model(&model.AuthzRole{}).Where(map[string]any{"key": BuiltInRoleAdmin}).Update("name", "preserve-on-failure").Error)
	require.NoError(t, ReloadPolicy())
	var rules []model.CasbinRule
	var roles []model.AuthzRole
	require.NoError(t, db.Order("id").Find(&rules).Error)
	require.NoError(t, db.Order("id").Find(&roles).Error)
	return rules, roles
}

func checkAuthzRows(t *testing.T, db *gorm.DB, rules []model.CasbinRule, roles []model.AuthzRole) {
	t.Helper()
	var gotRules []model.CasbinRule
	var gotRoles []model.AuthzRole
	require.NoError(t, db.Order("id").Find(&gotRules).Error)
	require.NoError(t, db.Order("id").Find(&gotRoles).Error)
	assert.Equal(t, rules, gotRules)
	assert.Equal(t, roles, gotRoles)
}

func runAuthzAtomicFailures(t *testing.T, open func(*testing.T) *gorm.DB) {
	for _, phase := range []string{"role", "delete", "first_insert", "later_insert", "load"} {
		t.Run(phase, func(t *testing.T) {
			db := open(t)
			rules, roles := authzAtomicFixture(t, db)
			previous := currentEnforcer()
			forced := errors.New("forced authz initialization failure")
			const callback = "test:authz_init_failure"
			inserts := 0
			fail := func(tx *gorm.DB) {
				if phase == "role" && tx.Statement.Table == "authz_roles" {
					tx.AddError(forced)
				}
				if tx.Statement.Table == "casbin_rule" {
					inserts++
					if phase == "first_insert" || (phase == "later_insert" && inserts == 2) {
						tx.AddError(forced)
					}
				}
			}
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, fail))
			require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
				if phase == "delete" && tx.Statement.Table == "casbin_rule" {
					tx.AddError(forced)
				}
			}))
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if phase == "load" && tx.Statement.Table == "casbin_rule" {
					tx.AddError(forced)
				}
			}))
			err := Init(db)
			require.NoError(t, db.Callback().Create().Remove(callback))
			require.NoError(t, db.Callback().Delete().Remove(callback))
			require.NoError(t, db.Callback().Query().Remove(callback))
			require.ErrorIs(t, err, forced)
			assert.Same(t, previous, currentEnforcer(), "failed init must not replace live enforcer")
			checkAuthzRows(t, db, rules, roles)
			require.NoError(t, Init(db), "restart after confirmed rollback restores complete baseline")
			assert.True(t, Can(2, common.RoleAdminUser, ChannelRead))
		})
	}
}

func TestAuthzInitializationAtomicFailures(t *testing.T) { runAuthzAtomicFailures(t, newAuthzTestDB) }

func TestAuthzInitializationPreservesPoliciesAndRebindsAdapter(t *testing.T) {
	db := newAuthzTestDB(t)
	rules, _ := authzAtomicFixture(t, db)
	var sentinels []model.CasbinRule
	for _, rule := range rules {
		if rule.Ptype != "p" || rule.V0 != RoleSubject(BuiltInRoleAdmin) {
			sentinels = append(sentinels, rule)
		}
	}
	require.NoError(t, Init(db))
	require.NoError(t, Init(db))
	var got []model.CasbinRule
	require.NoError(t, db.Where("ptype != ? OR v0 != ?", "p", RoleSubject(BuiltInRoleAdmin)).Order("id").Find(&got).Error)
	assert.Equal(t, sentinels, got)
	var count int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Where("ptype = ? AND v0 = ?", "p", RoleSubject(BuiltInRoleAdmin)).Count(&count).Error)
	assert.Equal(t, int64(len(PermissionsForRole(BuiltInRoleAdmin))), count)
	require.NoError(t, SetUserPermissions(43, PermissionsMap{ResourceChannel: {ActionSensitiveWrite: true}}))
	require.NoError(t, ReloadPolicy(), "adapter must use original DB, not completed transaction")
	assert.True(t, Can(43, common.RoleAdminUser, ChannelSensitiveWrite))
}

type authzOutcomePool struct {
	*sql.DB
	applied bool
	forced  error
	begins  int
}

func (p *authzOutcomePool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	p.begins++
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &authzOutcomeTx{Tx: tx, applied: p.applied, forced: p.forced}, nil
}

type authzOutcomeTx struct {
	*sql.Tx
	applied bool
	forced  error
}

func (tx *authzOutcomeTx) Commit() error {
	if !tx.applied {
		_ = tx.Tx.Rollback()
		return tx.forced
	}
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	return tx.forced
}
func TestAuthzInitializationUnknownCommitNeverPublishesOrReplays(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(map[bool]string{false: "not_applied", true: "applied"}[applied], func(t *testing.T) {
			db := newAuthzTestDB(t)
			rules, roles := authzAtomicFixture(t, db)
			previous := currentEnforcer()
			sqlDB, err := db.DB()
			require.NoError(t, err)
			pool := &authzOutcomePool{DB: sqlDB, applied: applied, forced: errors.New("commit acknowledgement lost")}
			oldPool, oldStatement := db.ConnPool, db.Statement.ConnPool
			db.ConnPool, db.Statement.ConnPool = pool, pool
			err = Init(db)
			db.ConnPool, db.Statement.ConnPool = oldPool, oldStatement
			require.ErrorIs(t, err, pool.forced)
			assert.Equal(t, 1, pool.begins)
			assert.Same(t, previous, currentEnforcer())
			if !applied {
				checkAuthzRows(t, db, rules, roles)
			} else {
				var count int64
				require.NoError(t, db.Model(&model.CasbinRule{}).Where("ptype = ? AND v0 = ?", "p", RoleSubject(BuiltInRoleAdmin)).Count(&count).Error)
				assert.Equal(t, int64(len(PermissionsForRole(BuiltInRoleAdmin))), count)
				var role model.AuthzRole
				require.NoError(t, db.Where(map[string]any{"key": BuiltInRoleAdmin}).First(&role).Error)
				assert.Equal(t, "Admin", role.Name)
				var preserved, gotPreserved []model.CasbinRule
				for _, rule := range rules {
					if rule.Ptype != "p" || rule.V0 != RoleSubject(BuiltInRoleAdmin) {
						preserved = append(preserved, rule)
					}
				}
				require.NoError(t, db.Where("ptype != ? OR v0 != ?", "p", RoleSubject(BuiltInRoleAdmin)).Order("id").Find(&gotPreserved).Error)
				assert.Equal(t, preserved, gotPreserved)
			}
		})
	}
}

func TestAuthzInitializationUnsupportedGroupingPreservesDatabaseAndEnforcer(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))
	previous := currentEnforcer()
	rule := newRule("g", []string{UserSubject(42), RoleSubject("custom")})
	require.NoError(t, db.Create(&rule).Error)
	var rules []model.CasbinRule
	var roles []model.AuthzRole
	require.NoError(t, db.Order("id").Find(&rules).Error)
	require.NoError(t, db.Order("id").Find(&roles).Error)
	require.ErrorContains(t, Init(db), "missing required section g")
	assert.Same(t, previous, currentEnforcer())
	checkAuthzRows(t, db, rules, roles)
}

func TestAuthzSlaveLoadFailureRetainsEnforcerAndNeverSeeds(t *testing.T) {
	db := newAuthzTestDB(t)
	rules, roles := authzAtomicFixture(t, db)
	previous := currentEnforcer()
	common.IsMasterNode = false
	forced := errors.New("slave policy read failure")
	const callback = "test:slave_policy_read"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "casbin_rule" {
			tx.AddError(forced)
		}
	}))
	err := Init(db)
	require.NoError(t, db.Callback().Query().Remove(callback))
	require.ErrorIs(t, err, forced)
	assert.Same(t, previous, currentEnforcer())
	checkAuthzRows(t, db, rules, roles)
	require.NoError(t, Init(db))
	checkAuthzRows(t, db, rules, roles)
}
