package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

// TestResumeSiteScope locks the cross-module contract between campaign creation and
// crash recovery: the scope reconstructed at resume time must match what each
// creation call site originally passed to StartEmailCampaign. Both call sites use
// SiteScopeAll today — manual campaigns are created behind AdminAuth (whose
// EffectiveSiteScope is always All) and announcement campaigns mail all users
// because console announcements are a single global option. The campaign's SiteId
// is informational and must NOT narrow the resumed audience.
func TestResumeSiteScope(t *testing.T) {
	manualSub := &model.EmailCampaign{Source: model.EmailCampaignSourceManual, SiteId: 7}
	assert.Equal(t, model.SiteScopeAll, resumeSiteScope(manualSub))

	annSub := &model.EmailCampaign{Source: model.EmailCampaignSourceAnnouncement, SiteId: 7}
	assert.Equal(t, model.SiteScopeAll, resumeSiteScope(annSub))
}
