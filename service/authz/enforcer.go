package authz

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"
)

var (
	initMu     sync.Mutex
	enforcerMu sync.RWMutex
	enforcer   *casbin.SyncedEnforcer
)

const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act && p.eft == "allow"
`

// Init builds a private complete snapshot. Built-in resets and seeding are one
// transaction; neither a failed statement nor an uncertain commit publishes it.
func Init(db *gorm.DB) error {
	initMu.Lock()
	defer initMu.Unlock()
	var candidate *casbin.SyncedEnforcer
	build := func(policyDB *gorm.DB) error {
		m, err := casbinmodel.NewModelFromString(modelText)
		if err != nil {
			return err
		}
		candidate, err = casbin.NewSyncedEnforcer(m, newGormAdapter(policyDB))
		return err
	}
	if common.IsMasterNode {
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := seedBuiltInRoles(tx); err != nil {
				return err
			}
			if err := resetBuiltInRolePolicies(tx); err != nil {
				return err
			}
			if err := seedDefaultPolicies(tx); err != nil {
				return err
			}
			return build(tx)
		}); err != nil {
			return err
		}
	} else if err := build(db); err != nil {
		return err
	}
	// Never retain the transaction adapter after its commit.
	candidate.SetAdapter(newGormAdapter(db))
	candidate.EnableAutoSave(true)
	enforcerMu.Lock()
	enforcer = candidate
	enforcerMu.Unlock()
	return nil
}

func currentEnforcer() *casbin.SyncedEnforcer {
	enforcerMu.RLock()
	defer enforcerMu.RUnlock()
	return enforcer
}

func ReloadPolicy() error {
	enforcerMu.Lock()
	defer enforcerMu.Unlock()
	if enforcer == nil {
		return fmt.Errorf("authz enforcer is not initialized")
	}
	return enforcer.LoadPolicy()
}

// StartPolicySync periodically reloads the authorization policy from the database.
// The enforcer keeps an in-memory snapshot, and permission changes are written
// straight to the DB (see SetUserPermissionsInTx) with only the local node's
// snapshot refreshed afterwards. Without this loop other instances in a
// multi-node deployment would keep serving stale permissions (including not
// honoring a revoked grant) until restart. Mirrors model.SyncOptions polling.
func StartPolicySync(frequency int) {
	if frequency <= 0 {
		return
	}
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		if err := ReloadPolicy(); err != nil {
			common.SysError("failed to reload authz policy: " + err.Error())
		}
	}
}
