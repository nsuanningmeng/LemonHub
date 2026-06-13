package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// SiteResolver identifies the sub-site (white-label tenant) for the current request
// from the Host header using an in-memory cache, and stores the resolved site id
// (0 for the main site) and *model.Site in the request context for downstream use.
//
// It is a pure resolver — it never blocks requests or alters relay/API behavior, so
// it is safe to register globally before all route groups.
func SiteResolver() func(c *gin.Context) {
	return func(c *gin.Context) {
		site := model.GetSiteByDomainCached(c.Request.Host)
		if site != nil {
			common.SetContextKey(c, constant.ContextKeySiteId, site.Id)
			common.SetContextKey(c, constant.ContextKeySite, site)
		} else {
			common.SetContextKey(c, constant.ContextKeySiteId, 0)
		}
		c.Next()
	}
}

// GetRequestSite returns the resolved *model.Site for the current request, or nil
// for the main site.
func GetRequestSite(c *gin.Context) *model.Site {
	if v, ok := common.GetContextKey(c, constant.ContextKeySite); ok {
		if site, ok := v.(*model.Site); ok {
			return site
		}
	}
	return nil
}

// GetRequestSiteId returns the resolved sub-site id for the current request (0 = main site).
func GetRequestSiteId(c *gin.Context) int {
	return common.GetContextKeyInt(c, constant.ContextKeySiteId)
}
