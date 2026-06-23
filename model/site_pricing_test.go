package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSiteModelPriceMultiplier proves the per-site model price markup resolves correctly and,
// critically, is CLAMPED to >= 1.0 so a sub-site can never price below the platform retail (the
// platform's wholesale revenue can never be undercut) even if a below-retail value is persisted.
func TestGetSiteModelPriceMultiplier(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Site{}, &SiteDomain{}))
	const sid = 7701
	clean := func() { DB.Unscoped().Where("id = ?", sid).Delete(&Site{}) }
	clean()
	t.Cleanup(func() { clean(); _ = ReloadSiteCache() })

	// Main site (0) and unknown sites always resolve to 1.0 (platform retail, behavior unchanged).
	assert.Equal(t, 1.0, GetSiteModelPriceMultiplier(0))
	assert.Equal(t, 1.0, GetSiteModelPriceMultiplier(999999))

	// A +30% markup resolves to 1.3.
	require.NoError(t, DB.Create(&Site{Id: sid, Name: "pricing", Status: SiteStatusNormal, ModelPriceRate: 13000}).Error)
	require.NoError(t, ReloadSiteCache())
	assert.InEpsilon(t, 1.3, GetSiteModelPriceMultiplier(sid), 1e-9)

	// Retail (10000) resolves to exactly 1.0.
	require.NoError(t, DB.Model(&Site{}).Where("id = ?", sid).Update("model_price_rate", DiscountRateBase).Error)
	require.NoError(t, ReloadSiteCache())
	assert.Equal(t, 1.0, GetSiteModelPriceMultiplier(sid))

	// Bad data below retail (e.g. 9000) MUST clamp to 1.0 — never below the platform price.
	require.NoError(t, DB.Model(&Site{}).Where("id = ?", sid).Update("model_price_rate", 9000).Error)
	require.NoError(t, ReloadSiteCache())
	assert.Equal(t, 1.0, GetSiteModelPriceMultiplier(sid), "below-retail rate must clamp to 1.0")
}

// TestUpdateSiteModelPriceRate_FloorAndCap proves the sub-site self-service setter enforces the
// floor (>= main retail) and the main-admin cap, so an agent can neither price below the platform
// nor exceed the admin-set ceiling.
func TestUpdateSiteModelPriceRate_FloorAndCap(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Site{}, &SiteDomain{}))
	const sid = 7702
	clean := func() { DB.Unscoped().Where("id = ?", sid).Delete(&Site{}) }
	clean()
	t.Cleanup(func() { clean(); _ = ReloadSiteCache() })

	require.NoError(t, DB.Create(&Site{Id: sid, Name: "pricing", Status: SiteStatusNormal, ModelPriceRate: DiscountRateBase, ModelPriceRateMax: 12000}).Error)

	require.Error(t, UpdateSiteModelPriceRate(sid, 9000), "below retail must be rejected")
	require.Error(t, UpdateSiteModelPriceRate(sid, 13000), "above cap must be rejected")

	require.NoError(t, UpdateSiteModelPriceRate(sid, 11500))
	got, err := GetSiteById(sid)
	require.NoError(t, err)
	assert.Equal(t, 11500, got.ModelPriceRate)

	require.NoError(t, UpdateSiteModelPriceRate(sid, DiscountRateBase), "exactly retail allowed")
	require.NoError(t, UpdateSiteModelPriceRate(sid, 12000), "exactly cap allowed")
}

// TestUpdateSitePreservesModelPriceRateWhenOmitted proves a main-admin site update that OMITS
// model_price_rate (sends 0) does NOT silently reset the sub-site's self-set markup back to retail.
func TestUpdateSitePreservesModelPriceRateWhenOmitted(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Site{}, &SiteDomain{}))
	const sid = 7703
	const dom = "preserve-test.example"
	clean := func() {
		DB.Unscoped().Where("id = ?", sid).Delete(&Site{})
		DB.Where("site_id = ?", sid).Delete(&SiteDomain{})
	}
	clean()
	t.Cleanup(func() { clean(); _ = ReloadSiteCache() })

	require.NoError(t, DB.Create(&Site{Id: sid, Name: "p", Status: SiteStatusNormal, DiscountRate: DiscountRateBase, ModelPriceRate: 13000}).Error)
	require.NoError(t, DB.Create(&SiteDomain{SiteId: sid, Domain: dom}).Error)

	// Main admin updates the site but OMITS model_price_rate (0) -> must be preserved (not reset).
	require.NoError(t, UpdateSite(&Site{
		Id:           sid,
		Name:         "p2",
		Status:       SiteStatusNormal,
		DiscountRate: DiscountRateBase,
		Domains:      []string{dom},
	}))

	got, err := GetSiteById(sid)
	require.NoError(t, err)
	assert.Equal(t, 13000, got.ModelPriceRate, "omitted model_price_rate must be preserved, not reset to retail")
	assert.Equal(t, "p2", got.Name)
}

