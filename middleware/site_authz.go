package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// SiteAdminAuth permits sub-site administrators (role >= RoleSubSiteAdmin) as well as
// main-site admins and root. Endpoints guarded by it MUST additionally scope every
// query/mutation to the operator's effective site (see EffectiveSiteScope) and reject
// cross-site resource access — the role gate alone does not provide isolation.
func SiteAdminAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleSubSiteAdmin)
	}
}

// EffectiveSiteScope returns the site_id an admin request is restricted to:
//   - main-site admins / root (role >= RoleAdminUser): model.SiteScopeAll (no filter)
//   - sub-site admins: the site they own (their account's site_id)
//
// The sub-site admin's own site_id is authoritative (taken from their session/account),
// never from the request Host, so it cannot be widened by changing domains.
func EffectiveSiteScope(c *gin.Context) int {
	if c.GetInt("role") >= common.RoleAdminUser {
		return model.SiteScopeAll
	}
	return c.GetInt("operator_site_id")
}
