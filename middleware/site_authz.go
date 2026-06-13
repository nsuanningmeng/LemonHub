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
//   - a scoped operator whose own site is unknown: model.SiteScopeDenied (fail closed)
//
// The sub-site admin's own site_id is authoritative (taken from their session/account
// in authHelper), never from the request Host, so it cannot be widened by changing
// domains. A missing operator_site_id maps to SiteScopeDenied rather than 0, so an
// unidentified scoped operator can never silently read main-site data.
func EffectiveSiteScope(c *gin.Context) int {
	if c.GetInt("role") >= common.RoleAdminUser {
		return model.SiteScopeAll
	}
	if v, ok := c.Get("operator_site_id"); ok {
		if siteId, ok := v.(int); ok {
			return siteId
		}
	}
	return model.SiteScopeDenied
}
